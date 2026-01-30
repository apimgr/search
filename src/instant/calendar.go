package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CalendarHandler handles date calculations and queries
type CalendarHandler struct {
	patterns   []*regexp.Regexp
	dateParser *dateParser
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler() *CalendarHandler {
	return &CalendarHandler{
		patterns: []*regexp.Regexp{
			// "days until {date}"
			regexp.MustCompile(`(?i)^days\s+until\s+(.+)$`),
			// "days between {date1} and {date2}"
			regexp.MustCompile(`(?i)^days\s+between\s+(.+)\s+and\s+(.+)$`),
			// "{date} + {days}" or "{date} - {days}"
			regexp.MustCompile(`(?i)^(.+?)\s*([+-])\s*(\d+)\s*days?$`),
			// "what day is {date}"
			regexp.MustCompile(`(?i)^what\s+day\s+(?:is|was|will\s+be)\s+(.+)\??$`),
			// "days from now to {date}"
			regexp.MustCompile(`(?i)^days\s+from\s+now\s+to\s+(.+)$`),
			// "how many days until {date}"
			regexp.MustCompile(`(?i)^how\s+many\s+days\s+until\s+(.+)\??$`),
			// "days since {date}"
			regexp.MustCompile(`(?i)^days\s+since\s+(.+)$`),
			// "{number} days from now"
			regexp.MustCompile(`(?i)^(\d+)\s+days?\s+from\s+(?:now|today)$`),
			// "{number} days ago"
			regexp.MustCompile(`(?i)^(\d+)\s+days?\s+ago$`),
			// "{number} weeks from now"
			regexp.MustCompile(`(?i)^(\d+)\s+weeks?\s+from\s+(?:now|today)$`),
			// "{number} weeks ago"
			regexp.MustCompile(`(?i)^(\d+)\s+weeks?\s+ago$`),
			// "{number} months from now"
			regexp.MustCompile(`(?i)^(\d+)\s+months?\s+from\s+(?:now|today)$`),
			// "{number} months ago"
			regexp.MustCompile(`(?i)^(\d+)\s+months?\s+ago$`),
		},
		dateParser: newDateParser(),
	}
}

func (h *CalendarHandler) Name() string {
	return "calendar"
}

func (h *CalendarHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *CalendarHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CalendarHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)

	// Handle "days until {date}" and "how many days until {date}"
	daysUntilPattern := regexp.MustCompile(`(?i)^(?:days\s+until|how\s+many\s+days\s+until|days\s+from\s+now\s+to)\s+(.+?)\??$`)
	if matches := daysUntilPattern.FindStringSubmatch(query); len(matches) > 1 {
		return h.handleDaysUntil(query, matches[1])
	}

	// Handle "days since {date}"
	daysSincePattern := regexp.MustCompile(`(?i)^days\s+since\s+(.+)$`)
	if matches := daysSincePattern.FindStringSubmatch(query); len(matches) > 1 {
		return h.handleDaysSince(query, matches[1])
	}

	// Handle "days between {date1} and {date2}"
	betweenPattern := regexp.MustCompile(`(?i)^days\s+between\s+(.+)\s+and\s+(.+)$`)
	if matches := betweenPattern.FindStringSubmatch(query); len(matches) > 2 {
		return h.handleDaysBetween(query, matches[1], matches[2])
	}

	// Handle "{date} + {days}" or "{date} - {days}"
	arithmeticPattern := regexp.MustCompile(`(?i)^(.+?)\s*([+-])\s*(\d+)\s*days?$`)
	if matches := arithmeticPattern.FindStringSubmatch(query); len(matches) > 3 {
		return h.handleDateArithmetic(query, matches[1], matches[2], matches[3])
	}

	// Handle "what day is {date}"
	whatDayPattern := regexp.MustCompile(`(?i)^what\s+day\s+(?:is|was|will\s+be)\s+(.+?)\??$`)
	if matches := whatDayPattern.FindStringSubmatch(query); len(matches) > 1 {
		return h.handleWhatDay(query, matches[1])
	}

	// Handle "{number} days from now"
	daysFromNowPattern := regexp.MustCompile(`(?i)^(\d+)\s+days?\s+from\s+(?:now|today)$`)
	if matches := daysFromNowPattern.FindStringSubmatch(query); len(matches) > 1 {
		days, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDate(query, days, "day")
	}

	// Handle "{number} days ago"
	daysAgoPattern := regexp.MustCompile(`(?i)^(\d+)\s+days?\s+ago$`)
	if matches := daysAgoPattern.FindStringSubmatch(query); len(matches) > 1 {
		days, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDate(query, -days, "day")
	}

	// Handle "{number} weeks from now"
	weeksFromNowPattern := regexp.MustCompile(`(?i)^(\d+)\s+weeks?\s+from\s+(?:now|today)$`)
	if matches := weeksFromNowPattern.FindStringSubmatch(query); len(matches) > 1 {
		weeks, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDate(query, weeks*7, "day")
	}

	// Handle "{number} weeks ago"
	weeksAgoPattern := regexp.MustCompile(`(?i)^(\d+)\s+weeks?\s+ago$`)
	if matches := weeksAgoPattern.FindStringSubmatch(query); len(matches) > 1 {
		weeks, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDate(query, -weeks*7, "day")
	}

	// Handle "{number} months from now"
	monthsFromNowPattern := regexp.MustCompile(`(?i)^(\d+)\s+months?\s+from\s+(?:now|today)$`)
	if matches := monthsFromNowPattern.FindStringSubmatch(query); len(matches) > 1 {
		months, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDateMonths(query, months)
	}

	// Handle "{number} months ago"
	monthsAgoPattern := regexp.MustCompile(`(?i)^(\d+)\s+months?\s+ago$`)
	if matches := monthsAgoPattern.FindStringSubmatch(query); len(matches) > 1 {
		months, _ := strconv.Atoi(matches[1])
		return h.handleRelativeDateMonths(query, -months)
	}

	// If we couldn't match anything specific, check if lowerQuery at least seems date-related
	_ = lowerQuery
	return nil, nil
}

func (h *CalendarHandler) handleDaysUntil(query, dateStr string) (*Answer, error) {
	targetDate, err := h.dateParser.parse(dateStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse date: %s", dateStr),
		}, nil
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	target := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, now.Location())

	days := int(target.Sub(today).Hours() / 24)

	var message string
	if days > 0 {
		message = fmt.Sprintf("There are <strong>%d days</strong> until %s", days, target.Format("Monday, January 2, 2006"))
	} else if days < 0 {
		message = fmt.Sprintf("%s was <strong>%d days</strong> ago", target.Format("Monday, January 2, 2006"), -days)
	} else {
		message = fmt.Sprintf("%s is <strong>today</strong>", target.Format("Monday, January 2, 2006"))
	}

	return &Answer{
		Type:    AnswerTypeDate,
		Query:   query,
		Title:   "Days Until",
		Content: fmt.Sprintf(`<div class="calendar-result">%s</div>`, message),
		Data: map[string]interface{}{
			"days":       days,
			"targetDate": target.Format("2006-01-02"),
		},
	}, nil
}

func (h *CalendarHandler) handleDaysSince(query, dateStr string) (*Answer, error) {
	targetDate, err := h.dateParser.parse(dateStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse date: %s", dateStr),
		}, nil
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	target := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, now.Location())

	days := int(today.Sub(target).Hours() / 24)

	var message string
	if days > 0 {
		message = fmt.Sprintf("It has been <strong>%d days</strong> since %s", days, target.Format("Monday, January 2, 2006"))
	} else if days < 0 {
		message = fmt.Sprintf("%s is <strong>%d days</strong> in the future", target.Format("Monday, January 2, 2006"), -days)
	} else {
		message = fmt.Sprintf("%s is <strong>today</strong>", target.Format("Monday, January 2, 2006"))
	}

	return &Answer{
		Type:    AnswerTypeDate,
		Query:   query,
		Title:   "Days Since",
		Content: fmt.Sprintf(`<div class="calendar-result">%s</div>`, message),
		Data: map[string]interface{}{
			"days":       days,
			"targetDate": target.Format("2006-01-02"),
		},
	}, nil
}

func (h *CalendarHandler) handleDaysBetween(query, dateStr1, dateStr2 string) (*Answer, error) {
	date1, err := h.dateParser.parse(dateStr1)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse first date: %s", dateStr1),
		}, nil
	}

	date2, err := h.dateParser.parse(dateStr2)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse second date: %s", dateStr2),
		}, nil
	}

	d1 := time.Date(date1.Year(), date1.Month(), date1.Day(), 0, 0, 0, 0, time.Local)
	d2 := time.Date(date2.Year(), date2.Month(), date2.Day(), 0, 0, 0, 0, time.Local)

	days := int(d2.Sub(d1).Hours() / 24)
	absDays := days
	if absDays < 0 {
		absDays = -absDays
	}

	return &Answer{
		Type:  AnswerTypeDate,
		Query: query,
		Title: "Days Between",
		Content: fmt.Sprintf(`<div class="calendar-result">
There are <strong>%d days</strong> between %s and %s
</div>`,
			absDays,
			d1.Format("Monday, January 2, 2006"),
			d2.Format("Monday, January 2, 2006")),
		Data: map[string]interface{}{
			"days":  absDays,
			"date1": d1.Format("2006-01-02"),
			"date2": d2.Format("2006-01-02"),
		},
	}, nil
}

func (h *CalendarHandler) handleDateArithmetic(query, dateStr, operator, daysStr string) (*Answer, error) {
	baseDate, err := h.dateParser.parse(dateStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse date: %s", dateStr),
		}, nil
	}

	days, _ := strconv.Atoi(daysStr)
	if operator == "-" {
		days = -days
	}

	resultDate := baseDate.AddDate(0, 0, days)

	return &Answer{
		Type:  AnswerTypeDate,
		Query: query,
		Title: "Date Arithmetic",
		Content: fmt.Sprintf(`<div class="calendar-result">
%s %s %s days = <strong>%s</strong>
</div>`,
			baseDate.Format("January 2, 2006"),
			operator,
			daysStr,
			resultDate.Format("Monday, January 2, 2006")),
		Data: map[string]interface{}{
			"baseDate":   baseDate.Format("2006-01-02"),
			"days":       days,
			"resultDate": resultDate.Format("2006-01-02"),
		},
	}, nil
}

func (h *CalendarHandler) handleWhatDay(query, dateStr string) (*Answer, error) {
	targetDate, err := h.dateParser.parse(dateStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeDate,
			Query:   query,
			Title:   "Date Calculator",
			Content: fmt.Sprintf("Could not parse date: %s", dateStr),
		}, nil
	}

	weekday := targetDate.Weekday().String()
	_, week := targetDate.ISOWeek()

	return &Answer{
		Type:  AnswerTypeDate,
		Query: query,
		Title: "Weekday Lookup",
		Content: fmt.Sprintf(`<div class="calendar-result">
%s is a <strong>%s</strong><br>
<small>Week %d of %d</small>
</div>`,
			targetDate.Format("January 2, 2006"),
			weekday,
			week,
			targetDate.Year()),
		Data: map[string]interface{}{
			"date":       targetDate.Format("2006-01-02"),
			"weekday":    weekday,
			"weekNumber": week,
		},
	}, nil
}

func (h *CalendarHandler) handleRelativeDate(query string, days int, unit string) (*Answer, error) {
	now := time.Now()
	resultDate := now.AddDate(0, 0, days)

	var description string
	if days >= 0 {
		description = fmt.Sprintf("%d %ss from now", days, unit)
	} else {
		description = fmt.Sprintf("%d %ss ago", -days, unit)
	}

	return &Answer{
		Type:  AnswerTypeDate,
		Query: query,
		Title: "Date Calculator",
		Content: fmt.Sprintf(`<div class="calendar-result">
<strong>%s</strong> is <strong>%s</strong>
</div>`,
			description,
			resultDate.Format("Monday, January 2, 2006")),
		Data: map[string]interface{}{
			"days":       days,
			"resultDate": resultDate.Format("2006-01-02"),
		},
	}, nil
}

func (h *CalendarHandler) handleRelativeDateMonths(query string, months int) (*Answer, error) {
	now := time.Now()
	resultDate := now.AddDate(0, months, 0)

	var description string
	absMonths := months
	if absMonths < 0 {
		absMonths = -absMonths
	}
	if months >= 0 {
		description = fmt.Sprintf("%d month(s) from now", absMonths)
	} else {
		description = fmt.Sprintf("%d month(s) ago", absMonths)
	}

	return &Answer{
		Type:  AnswerTypeDate,
		Query: query,
		Title: "Date Calculator",
		Content: fmt.Sprintf(`<div class="calendar-result">
<strong>%s</strong> is <strong>%s</strong>
</div>`,
			description,
			resultDate.Format("Monday, January 2, 2006")),
		Data: map[string]interface{}{
			"months":     months,
			"resultDate": resultDate.Format("2006-01-02"),
		},
	}, nil
}

// dateParser handles parsing various date formats
type dateParser struct {
	formats []string
	months  map[string]time.Month
}

func newDateParser() *dateParser {
	return &dateParser{
		formats: []string{
			"2006-01-02",          // ISO format
			"01/02/2006",          // US format
			"02/01/2006",          // UK format (try after US)
			"January 2, 2006",     // Full month name
			"Jan 2, 2006",         // Abbreviated month
			"January 2 2006",      // Full month name without comma
			"Jan 2 2006",          // Abbreviated month without comma
			"2 January 2006",      // European format
			"2 Jan 2006",          // European abbreviated
			"01-02-2006",          // Dashed US format
			"2006/01/02",          // ISO with slashes
			"January 2",           // Month day (current year)
			"Jan 2",               // Abbreviated month day
			"01/02",               // MM/DD (current year)
		},
		months: map[string]time.Month{
			"january":   time.January,
			"jan":       time.January,
			"february":  time.February,
			"feb":       time.February,
			"march":     time.March,
			"mar":       time.March,
			"april":     time.April,
			"apr":       time.April,
			"may":       time.May,
			"june":      time.June,
			"jun":       time.June,
			"july":      time.July,
			"jul":       time.July,
			"august":    time.August,
			"aug":       time.August,
			"september": time.September,
			"sep":       time.September,
			"sept":      time.September,
			"october":   time.October,
			"oct":       time.October,
			"november":  time.November,
			"nov":       time.November,
			"december":  time.December,
			"dec":       time.December,
		},
	}
}

func (p *dateParser) parse(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	// Handle special keywords
	now := time.Now()
	lowerDate := strings.ToLower(dateStr)

	switch lowerDate {
	case "today":
		return now, nil
	case "tomorrow":
		return now.AddDate(0, 0, 1), nil
	case "yesterday":
		return now.AddDate(0, 0, -1), nil
	}

	// Handle "next {weekday}" or "last {weekday}"
	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,
	}

	for name, wd := range weekdays {
		if strings.HasPrefix(lowerDate, "next ") && strings.Contains(lowerDate, name) {
			return p.nextWeekday(now, wd), nil
		}
		if strings.HasPrefix(lowerDate, "last ") && strings.Contains(lowerDate, name) {
			return p.lastWeekday(now, wd), nil
		}
		if lowerDate == name {
			// Assume next occurrence of that weekday
			return p.nextWeekday(now, wd), nil
		}
	}

	// Handle holidays
	holidays := map[string]func(int) time.Time{
		"christmas":      func(y int) time.Time { return time.Date(y, time.December, 25, 0, 0, 0, 0, time.Local) },
		"christmas day":  func(y int) time.Time { return time.Date(y, time.December, 25, 0, 0, 0, 0, time.Local) },
		"new year":       func(y int) time.Time { return time.Date(y+1, time.January, 1, 0, 0, 0, 0, time.Local) },
		"new years":      func(y int) time.Time { return time.Date(y+1, time.January, 1, 0, 0, 0, 0, time.Local) },
		"new years day":  func(y int) time.Time { return time.Date(y+1, time.January, 1, 0, 0, 0, 0, time.Local) },
		"new year's day": func(y int) time.Time { return time.Date(y+1, time.January, 1, 0, 0, 0, 0, time.Local) },
		"valentines":     func(y int) time.Time { return time.Date(y, time.February, 14, 0, 0, 0, 0, time.Local) },
		"valentines day": func(y int) time.Time { return time.Date(y, time.February, 14, 0, 0, 0, 0, time.Local) },
		"halloween":      func(y int) time.Time { return time.Date(y, time.October, 31, 0, 0, 0, 0, time.Local) },
		"independence day": func(y int) time.Time { return time.Date(y, time.July, 4, 0, 0, 0, 0, time.Local) },
		"july 4th":       func(y int) time.Time { return time.Date(y, time.July, 4, 0, 0, 0, 0, time.Local) },
	}

	for name, holidayFn := range holidays {
		if lowerDate == name {
			date := holidayFn(now.Year())
			// If the holiday has passed this year, return next year's date
			if date.Before(now) {
				date = holidayFn(now.Year() + 1)
			}
			return date, nil
		}
	}

	// Try standard formats
	for _, format := range p.formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			// If year is not specified, use current year
			if t.Year() == 0 {
				t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
				// If the date has passed this year, use next year
				if t.Before(now) {
					t = t.AddDate(1, 0, 0)
				}
			}
			return t, nil
		}
	}

	// Try with case-insensitive month parsing
	// Pattern: "Month Day, Year" or "Month Day Year"
	monthPattern := regexp.MustCompile(`(?i)^(\w+)\s+(\d{1,2})(?:,?\s+(\d{4}))?$`)
	if matches := monthPattern.FindStringSubmatch(dateStr); len(matches) >= 3 {
		monthStr := strings.ToLower(matches[1])
		if month, ok := p.months[monthStr]; ok {
			day, _ := strconv.Atoi(matches[2])
			year := now.Year()
			if len(matches) > 3 && matches[3] != "" {
				year, _ = strconv.Atoi(matches[3])
			}
			t := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
			// If no year was specified and the date has passed, use next year
			if (len(matches) < 4 || matches[3] == "") && t.Before(now) {
				t = t.AddDate(1, 0, 0)
			}
			return t, nil
		}
	}

	// Pattern: "Day Month Year" (European)
	eurPattern := regexp.MustCompile(`(?i)^(\d{1,2})\s+(\w+)(?:\s+(\d{4}))?$`)
	if matches := eurPattern.FindStringSubmatch(dateStr); len(matches) >= 3 {
		day, _ := strconv.Atoi(matches[1])
		monthStr := strings.ToLower(matches[2])
		if month, ok := p.months[monthStr]; ok {
			year := now.Year()
			if len(matches) > 3 && matches[3] != "" {
				year, _ = strconv.Atoi(matches[3])
			}
			t := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
			if (len(matches) < 4 || matches[3] == "") && t.Before(now) {
				t = t.AddDate(1, 0, 0)
			}
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
}

func (p *dateParser) nextWeekday(from time.Time, wd time.Weekday) time.Time {
	daysUntil := int(wd - from.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}
	return from.AddDate(0, 0, daysUntil)
}

func (p *dateParser) lastWeekday(from time.Time, wd time.Weekday) time.Time {
	daysSince := int(from.Weekday() - wd)
	if daysSince <= 0 {
		daysSince += 7
	}
	return from.AddDate(0, 0, -daysSince)
}
