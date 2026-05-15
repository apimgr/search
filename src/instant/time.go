package instant

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// TimeHandler handles time/date queries
type TimeHandler struct {
	patterns []*regexp.Regexp
}

func NewTimeHandler() *TimeHandler {
	return &TimeHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^time\s*$`),
			regexp.MustCompile(`(?i)^current\s+time\s*$`),
			regexp.MustCompile(`(?i)^what\s+time\s+is\s+it\s*\??$`),
			regexp.MustCompile(`(?i)^now\s*$`),
			regexp.MustCompile(`(?i)^date\s*$`),
			regexp.MustCompile(`(?i)^today\s*$`),
			regexp.MustCompile(`(?i)^current\s+date\s*$`),
			regexp.MustCompile(`(?i)^timestamp\s*$`),
			regexp.MustCompile(`(?i)^unix\s*time(?:stamp)?\s*$`),
			regexp.MustCompile(`(?i)^epoch\s*$`),
		},
	}
}

func (h *TimeHandler) Name() string                { return "time" }
func (h *TimeHandler) Patterns() []*regexp.Regexp  { return h.patterns }

func (h *TimeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TimeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	now := time.Now()
	utc := now.UTC()

	content := fmt.Sprintf(`<div class="time-result">
<strong>Local Time:</strong> %s<br>
<strong>UTC Time:</strong> %s<br>
<strong>Unix Timestamp:</strong> %d<br>
<strong>ISO 8601:</strong> %s<br>
<strong>Day of Year:</strong> %d<br>
<strong>Week of Year:</strong> %d
</div>`,
		now.Format("Monday, January 2, 2006 3:04:05 PM MST"),
		utc.Format("Monday, January 2, 2006 15:04:05 UTC"),
		now.Unix(),
		now.Format(time.RFC3339),
		now.YearDay(),
		getWeekOfYear(now),
	)

	return &Answer{
		Type:    AnswerTypeTime,
		Query:   query,
		Title:   "Current Time",
		Content: content,
		Data: map[string]interface{}{
			"local":     now.Format(time.RFC3339),
			"utc":       utc.Format(time.RFC3339),
			"timestamp": now.Unix(),
		},
	}, nil
}

func getWeekOfYear(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}
