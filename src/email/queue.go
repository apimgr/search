package email

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Queue defaults per AI.md PART 17 compliance checklist ("Email queue with
// retry logic", "Email logging (success/failure)").
const (
	// DefaultQueueSize is the number of messages that may wait for delivery
	// before Enqueue reports the queue is full. Bounded on purpose: an
	// unbounded queue would grow without limit while SMTP is unreachable.
	DefaultQueueSize = 256
	// DefaultQueueWorkers is the number of concurrent delivery workers.
	DefaultQueueWorkers = 1
	// DefaultMaxAttempts is the total number of delivery attempts made for a
	// single message (first attempt plus retries).
	DefaultMaxAttempts = 3
	// DefaultInitialBackoff is the delay before the first retry. Each further
	// retry doubles the delay up to DefaultMaxBackoff.
	DefaultInitialBackoff = 5 * time.Second
	// DefaultMaxBackoff caps the exponential retry backoff.
	DefaultMaxBackoff = 5 * time.Minute
)

// ErrQueueFull is returned by Enqueue when the delivery queue is at capacity.
var ErrQueueFull = errors.New("email queue is full")

// ErrQueueStopped is returned by Enqueue after the queue has been stopped.
var ErrQueueStopped = errors.New("email queue is stopped")

// QueueConfig tunes the delivery queue. A zero value selects the defaults
// above for every field.
type QueueConfig struct {
	// Size is the queue capacity in messages.
	Size int
	// Workers is the number of concurrent delivery workers.
	Workers int
	// MaxAttempts is the total number of delivery attempts per message.
	MaxAttempts int
	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponential retry backoff.
	MaxBackoff time.Duration
}

// withDefaults returns a copy of the config with every unset field replaced by
// its default.
func (qc QueueConfig) withDefaults() QueueConfig {
	if qc.Size <= 0 {
		qc.Size = DefaultQueueSize
	}
	if qc.Workers <= 0 {
		qc.Workers = DefaultQueueWorkers
	}
	if qc.MaxAttempts <= 0 {
		qc.MaxAttempts = DefaultMaxAttempts
	}
	if qc.InitialBackoff <= 0 {
		qc.InitialBackoff = DefaultInitialBackoff
	}
	if qc.MaxBackoff < qc.InitialBackoff {
		qc.MaxBackoff = DefaultMaxBackoff
	}
	if qc.MaxBackoff < qc.InitialBackoff {
		qc.MaxBackoff = qc.InitialBackoff
	}
	return qc
}

// QueueStats reports counters for the delivery queue.
type QueueStats struct {
	// Queued is the number of messages accepted for delivery.
	Queued uint64 `json:"queued"`
	// Sent is the number of messages delivered successfully.
	Sent uint64 `json:"sent"`
	// Failed is the number of messages dropped after exhausting all attempts.
	Failed uint64 `json:"failed"`
	// Retried is the number of retry attempts made across all messages.
	Retried uint64 `json:"retried"`
	// Pending is the number of messages currently waiting for a worker.
	Pending int `json:"pending"`
	// Running reports whether the delivery workers are active.
	Running bool `json:"running"`
}

// queueItem is a single message awaiting delivery.
type queueItem struct {
	msg *Message
}

// Queue is a bounded in-memory send queue with retry and backoff. It exists
// only while SMTP is configured: per AI.md PART 17 emails are never queued in
// the hope that SMTP is configured later, so Mailer.StartQueue refuses to
// start a queue for a mailer that cannot send.
type Queue struct {
	mailer *Mailer
	cfg    QueueConfig
	items  chan queueItem
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	running bool
	stats   QueueStats
}

// StartQueue starts the mailer's delivery queue with the given configuration.
// A zero QueueConfig selects the package defaults. It is a no-op returning nil
// when a queue is already running. It returns an error when the mailer has no
// working SMTP configuration, because AI.md PART 17 forbids queuing mail that
// cannot be delivered.
func (ml *Mailer) StartQueue(ctx context.Context, qc QueueConfig) error {
	if !ml.CanSend() {
		return errors.New("SMTP is not configured; email queue not started")
	}

	ml.queueMu.Lock()
	defer ml.queueMu.Unlock()
	if ml.queue != nil && ml.queue.IsRunning() {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)

	cfg := qc.withDefaults()
	q := &Queue{
		mailer: ml,
		cfg:    cfg,
		items:  make(chan queueItem, cfg.Size),
		cancel: cancel,
	}
	q.running = true

	for i := 0; i < cfg.Workers; i++ {
		q.wg.Add(1)
		go q.worker(runCtx)
	}

	ml.queue = q
	slog.Info("Email queue started", "workers", cfg.Workers, "size", cfg.Size, "max_attempts", cfg.MaxAttempts)
	return nil
}

// StopQueue drains and stops the mailer's delivery queue. Messages already
// accepted are attempted before the workers exit. It is safe to call when no
// queue is running.
func (ml *Mailer) StopQueue() {
	ml.queueMu.Lock()
	q := ml.queue
	ml.queue = nil
	ml.queueMu.Unlock()

	if q == nil {
		return
	}
	q.stop()
}

// QueueStats returns the current delivery queue counters. Running is false
// when no queue has been started.
func (ml *Mailer) QueueStats() QueueStats {
	q := ml.activeQueue()
	if q == nil {
		return QueueStats{}
	}
	return q.Stats()
}

// activeQueue returns the running queue, or nil when delivery is synchronous.
func (ml *Mailer) activeQueue() *Queue {
	ml.queueMu.RLock()
	q := ml.queue
	ml.queueMu.RUnlock()
	if q == nil || !q.IsRunning() {
		return nil
	}
	return q
}

// stop closes the queue and waits for the workers to finish draining it.
func (q *Queue) stop() {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return
	}
	q.running = false
	q.mu.Unlock()

	close(q.items)
	q.wg.Wait()
	q.cancel()
	slog.Info("Email queue stopped")
}

// IsRunning reports whether the queue is accepting messages.
func (q *Queue) IsRunning() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running
}

// Stats returns a snapshot of the queue counters.
func (q *Queue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.stats
	s.Pending = len(q.items)
	s.Running = q.running
	return s
}

// enqueue accepts a message for asynchronous delivery.
func (q *Queue) enqueue(msg *Message) error {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return ErrQueueStopped
	}
	q.mu.Unlock()

	select {
	case q.items <- queueItem{msg: msg}:
		q.mu.Lock()
		q.stats.Queued++
		q.mu.Unlock()
		return nil
	default:
		slog.Error("Email dropped: queue full", "subject", msg.Subject, "recipients", len(msg.To), "size", q.cfg.Size)
		return ErrQueueFull
	}
}

// worker delivers queued messages, retrying with exponential backoff and
// logging every success and every failure (AI.md PART 17 compliance
// checklist: "Email logging (success/failure)").
func (q *Queue) worker(ctx context.Context) {
	defer q.wg.Done()
	for item := range q.items {
		q.deliver(ctx, item.msg)
	}
}

// deliver attempts one message until it succeeds or the attempt budget is
// exhausted.
func (q *Queue) deliver(ctx context.Context, msg *Message) {
	backoff := q.cfg.InitialBackoff
	recipients := strings.Join(msg.To, ", ")

	for attempt := 1; attempt <= q.cfg.MaxAttempts; attempt++ {
		err := q.mailer.SendNow(msg)
		if err == nil {
			q.mu.Lock()
			q.stats.Sent++
			q.mu.Unlock()
			slog.Info("Email sent", "to", recipients, "subject", msg.Subject, "attempt", attempt)
			return
		}

		if attempt == q.cfg.MaxAttempts {
			q.mu.Lock()
			q.stats.Failed++
			q.mu.Unlock()
			slog.Error("Email delivery failed", "to", recipients, "subject", msg.Subject, "attempts", attempt, "err", err)
			return
		}

		q.mu.Lock()
		q.stats.Retried++
		q.mu.Unlock()
		slog.Warn("Email delivery attempt failed, retrying", "to", recipients, "subject", msg.Subject, "attempt", attempt, "retry_in", backoff, "err", err)

		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.stats.Failed++
			q.mu.Unlock()
			slog.Error("Email delivery abandoned: shutting down", "to", recipients, "subject", msg.Subject, "attempts", attempt)
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > q.cfg.MaxBackoff {
			backoff = q.cfg.MaxBackoff
		}
	}
}
