package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StopwatchHandler handles timer and stopwatch queries
type StopwatchHandler struct {
	patterns []*regexp.Regexp
}

// NewStopwatchHandler creates a new stopwatch handler
func NewStopwatchHandler() *StopwatchHandler {
	return &StopwatchHandler{
		patterns: []*regexp.Regexp{
			// "timer {duration}"
			regexp.MustCompile(`(?i)^timer\s+(.+)$`),
			// "set timer for {duration}"
			regexp.MustCompile(`(?i)^set\s+(?:a\s+)?timer\s+(?:for\s+)?(.+)$`),
			// "{duration} timer"
			regexp.MustCompile(`(?i)^(.+?)\s+timer$`),
			// "countdown {duration}"
			regexp.MustCompile(`(?i)^countdown\s+(.+)$`),
			// "stopwatch"
			regexp.MustCompile(`(?i)^stopwatch\s*$`),
			// "start stopwatch"
			regexp.MustCompile(`(?i)^start\s+(?:a\s+)?stopwatch\s*$`),
			// "pomodoro"
			regexp.MustCompile(`(?i)^pomodoro\s*$`),
			// "pomodoro timer"
			regexp.MustCompile(`(?i)^pomodoro\s+timer\s*$`),
			// "start pomodoro"
			regexp.MustCompile(`(?i)^start\s+(?:a\s+)?pomodoro\s*$`),
			// "work timer"
			regexp.MustCompile(`(?i)^work\s+timer\s*$`),
			// "break timer"
			regexp.MustCompile(`(?i)^break\s+timer\s*$`),
			// "alarm {duration}"
			regexp.MustCompile(`(?i)^alarm\s+(?:in\s+)?(.+)$`),
		},
	}
}

func (h *StopwatchHandler) Name() string {
	return "stopwatch"
}

func (h *StopwatchHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *StopwatchHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *StopwatchHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)

	// Handle stopwatch (no duration)
	if strings.Contains(lowerQuery, "stopwatch") && !strings.Contains(lowerQuery, "timer") {
		return h.handleStopwatch(query)
	}

	// Handle pomodoro
	if strings.Contains(lowerQuery, "pomodoro") {
		return h.handlePomodoro(query)
	}

	// Handle work timer
	if lowerQuery == "work timer" {
		return h.handleWorkTimer(query)
	}

	// Handle break timer
	if lowerQuery == "break timer" {
		return h.handleBreakTimer(query)
	}

	// Handle timer with duration
	var durationStr string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			durationStr = strings.TrimSpace(matches[1])
			break
		}
	}

	if durationStr != "" {
		return h.handleTimer(query, durationStr)
	}

	return nil, nil
}

func (h *StopwatchHandler) handleStopwatch(query string) (*Answer, error) {
	return &Answer{
		Type:  AnswerTypeTime,
		Query: query,
		Title: "Stopwatch",
		Content: `<div class="stopwatch-widget" data-type="stopwatch">
<div class="stopwatch-display">00:00:00</div>
<div class="stopwatch-controls">
<button class="stopwatch-start">Start</button>
<button class="stopwatch-stop" disabled>Stop</button>
<button class="stopwatch-reset" disabled>Reset</button>
<button class="stopwatch-lap">Lap</button>
</div>
<div class="stopwatch-laps"></div>
</div>`,
		Data: map[string]interface{}{
			"type":       "stopwatch",
			"interactive": true,
		},
	}, nil
}

func (h *StopwatchHandler) handlePomodoro(query string) (*Answer, error) {
	workDuration := 25 * time.Minute
	breakDuration := 5 * time.Minute
	longBreakDuration := 15 * time.Minute

	return &Answer{
		Type:  AnswerTypeTime,
		Query: query,
		Title: "Pomodoro Timer",
		Content: fmt.Sprintf(`<div class="timer-widget pomodoro-widget" data-type="pomodoro">
<div class="pomodoro-phase">Work Session</div>
<div class="timer-display">25:00</div>
<div class="timer-controls">
<button class="timer-start">Start</button>
<button class="timer-pause" disabled>Pause</button>
<button class="timer-reset">Reset</button>
</div>
<div class="pomodoro-info">
<p><strong>Pomodoro Technique:</strong></p>
<ul>
<li>Work for 25 minutes</li>
<li>Take a 5-minute break</li>
<li>After 4 pomodoros, take a 15-minute break</li>
</ul>
</div>
<div class="pomodoro-counter">Pomodoros completed: <span class="count">0</span></div>
</div>`),
		Data: map[string]interface{}{
			"type":              "pomodoro",
			"workDuration":      int(workDuration.Seconds()),
			"breakDuration":     int(breakDuration.Seconds()),
			"longBreakDuration": int(longBreakDuration.Seconds()),
			"pomodorosForLongBreak": 4,
			"interactive":       true,
		},
	}, nil
}

func (h *StopwatchHandler) handleWorkTimer(query string) (*Answer, error) {
	duration := 25 * time.Minute
	return h.createTimerAnswer(query, duration, "Work Timer", "Focus time! Get to work.")
}

func (h *StopwatchHandler) handleBreakTimer(query string) (*Answer, error) {
	duration := 5 * time.Minute
	return h.createTimerAnswer(query, duration, "Break Timer", "Time for a short break!")
}

func (h *StopwatchHandler) handleTimer(query, durationStr string) (*Answer, error) {
	duration, err := h.parseDuration(durationStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Timer",
			Content: fmt.Sprintf("Could not parse duration: %s<br><br>Try formats like: 5m, 1h30m, 30s, 1 hour 30 minutes", durationStr),
		}, nil
	}

	return h.createTimerAnswer(query, duration, "Timer", fmt.Sprintf("Timer set for %s", h.formatDuration(duration)))
}

func (h *StopwatchHandler) createTimerAnswer(query string, duration time.Duration, title, message string) (*Answer, error) {
	totalSeconds := int(duration.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	displayTime := fmt.Sprintf("%02d:%02d", minutes, seconds)
	if hours > 0 {
		displayTime = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}

	endTime := time.Now().Add(duration)

	return &Answer{
		Type:  AnswerTypeTime,
		Query: query,
		Title: title,
		Content: fmt.Sprintf(`<div class="timer-widget" data-type="timer" data-duration="%d">
<div class="timer-message">%s</div>
<div class="timer-display">%s</div>
<div class="timer-end-time">Will complete at: %s</div>
<div class="timer-controls">
<button class="timer-start">Start</button>
<button class="timer-pause" disabled>Pause</button>
<button class="timer-reset">Reset</button>
</div>
<div class="timer-progress">
<div class="progress-bar" style="width: 100%%"></div>
</div>
</div>`,
			totalSeconds,
			message,
			displayTime,
			endTime.Format("3:04 PM")),
		Data: map[string]interface{}{
			"type":          "timer",
			"durationSeconds": totalSeconds,
			"displayTime":   displayTime,
			"endTime":       endTime.Format(time.RFC3339),
			"interactive":   true,
		},
	}, nil
}

func (h *StopwatchHandler) parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))

	// Try Go's standard duration parsing first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle natural language durations
	var totalDuration time.Duration

	// Pattern for "X hours Y minutes Z seconds"
	patterns := []struct {
		pattern *regexp.Regexp
		unit    time.Duration
	}{
		{regexp.MustCompile(`(\d+)\s*(?:hours?|hrs?|h)`), time.Hour},
		{regexp.MustCompile(`(\d+)\s*(?:minutes?|mins?|m)`), time.Minute},
		{regexp.MustCompile(`(\d+)\s*(?:seconds?|secs?|s)`), time.Second},
	}

	matched := false
	for _, p := range patterns {
		if matches := p.pattern.FindAllStringSubmatch(s, -1); matches != nil {
			for _, match := range matches {
				if len(match) > 1 {
					val, _ := strconv.Atoi(match[1])
					totalDuration += time.Duration(val) * p.unit
					matched = true
				}
			}
		}
	}

	if matched {
		return totalDuration, nil
	}

	// Try parsing as just a number (assume minutes)
	if val, err := strconv.Atoi(s); err == nil {
		return time.Duration(val) * time.Minute, nil
	}

	// Handle "X:Y" format (minutes:seconds)
	colonPattern := regexp.MustCompile(`^(\d+):(\d{1,2})$`)
	if matches := colonPattern.FindStringSubmatch(s); len(matches) == 3 {
		mins, _ := strconv.Atoi(matches[1])
		secs, _ := strconv.Atoi(matches[2])
		return time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second, nil
	}

	// Handle "X:Y:Z" format (hours:minutes:seconds)
	longColonPattern := regexp.MustCompile(`^(\d+):(\d{1,2}):(\d{1,2})$`)
	if matches := longColonPattern.FindStringSubmatch(s); len(matches) == 4 {
		hours, _ := strconv.Atoi(matches[1])
		mins, _ := strconv.Atoi(matches[2])
		secs, _ := strconv.Atoi(matches[3])
		return time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second, nil
	}

	return 0, fmt.Errorf("could not parse duration: %s", s)
}

func (h *StopwatchHandler) formatDuration(d time.Duration) string {
	h2 := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	var parts []string
	if h2 > 0 {
		if h2 == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", h2))
		}
	}
	if m > 0 {
		if m == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", m))
		}
	}
	if s > 0 {
		if s == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", s))
		}
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	return strings.Join(parts, " ")
}
