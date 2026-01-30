package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimezoneHandler handles timezone queries and conversions
type TimezoneHandler struct {
	patterns  []*regexp.Regexp
	cityToTZ  map[string]string
	tzAliases map[string]string
}

// NewTimezoneHandler creates a new timezone handler
func NewTimezoneHandler() *TimezoneHandler {
	return &TimezoneHandler{
		patterns: []*regexp.Regexp{
			// "time in {city}"
			regexp.MustCompile(`(?i)^time\s+in\s+(.+)$`),
			// "what time is it in {city}"
			regexp.MustCompile(`(?i)^what\s+time\s+(?:is\s+it\s+)?in\s+(.+)\??$`),
			// "current time in {city}"
			regexp.MustCompile(`(?i)^current\s+time\s+in\s+(.+)$`),
			// "{time} {tz1} to {tz2}"
			regexp.MustCompile(`(?i)^(\d{1,2}(?::\d{2})?(?:\s*[ap]m)?)\s+([a-zA-Z]+(?:/[a-zA-Z_]+)?)\s+(?:to|in)\s+([a-zA-Z]+(?:/[a-zA-Z_]+)?)$`),
			// "convert {time} {tz1} to {tz2}"
			regexp.MustCompile(`(?i)^convert\s+(\d{1,2}(?::\d{2})?(?:\s*[ap]m)?)\s+([a-zA-Z]+(?:/[a-zA-Z_]+)?)\s+to\s+([a-zA-Z]+(?:/[a-zA-Z_]+)?)$`),
			// "{city} time"
			regexp.MustCompile(`(?i)^(.+?)\s+time$`),
		},
		cityToTZ: map[string]string{
			// North America
			"new york":      "America/New_York",
			"nyc":           "America/New_York",
			"los angeles":   "America/Los_Angeles",
			"la":            "America/Los_Angeles",
			"chicago":       "America/Chicago",
			"houston":       "America/Chicago",
			"phoenix":       "America/Phoenix",
			"philadelphia":  "America/New_York",
			"san antonio":   "America/Chicago",
			"san diego":     "America/Los_Angeles",
			"dallas":        "America/Chicago",
			"san jose":      "America/Los_Angeles",
			"san francisco": "America/Los_Angeles",
			"sf":            "America/Los_Angeles",
			"seattle":       "America/Los_Angeles",
			"denver":        "America/Denver",
			"boston":        "America/New_York",
			"detroit":       "America/Detroit",
			"miami":         "America/New_York",
			"atlanta":       "America/New_York",
			"toronto":       "America/Toronto",
			"vancouver":     "America/Vancouver",
			"montreal":      "America/Montreal",
			"mexico city":   "America/Mexico_City",

			// Europe
			"london":     "Europe/London",
			"paris":      "Europe/Paris",
			"berlin":     "Europe/Berlin",
			"madrid":     "Europe/Madrid",
			"rome":       "Europe/Rome",
			"amsterdam":  "Europe/Amsterdam",
			"brussels":   "Europe/Brussels",
			"vienna":     "Europe/Vienna",
			"zurich":     "Europe/Zurich",
			"stockholm":  "Europe/Stockholm",
			"oslo":       "Europe/Oslo",
			"copenhagen": "Europe/Copenhagen",
			"helsinki":   "Europe/Helsinki",
			"dublin":     "Europe/Dublin",
			"lisbon":     "Europe/Lisbon",
			"prague":     "Europe/Prague",
			"warsaw":     "Europe/Warsaw",
			"budapest":   "Europe/Budapest",
			"athens":     "Europe/Athens",
			"moscow":     "Europe/Moscow",
			"istanbul":   "Europe/Istanbul",

			// Asia
			"tokyo":      "Asia/Tokyo",
			"beijing":    "Asia/Shanghai",
			"shanghai":   "Asia/Shanghai",
			"hong kong":  "Asia/Hong_Kong",
			"singapore":  "Asia/Singapore",
			"seoul":      "Asia/Seoul",
			"mumbai":     "Asia/Kolkata",
			"delhi":      "Asia/Kolkata",
			"bangalore":  "Asia/Kolkata",
			"chennai":    "Asia/Kolkata",
			"kolkata":    "Asia/Kolkata",
			"dubai":      "Asia/Dubai",
			"bangkok":    "Asia/Bangkok",
			"jakarta":    "Asia/Jakarta",
			"manila":     "Asia/Manila",
			"taipei":     "Asia/Taipei",
			"kuala lumpur": "Asia/Kuala_Lumpur",
			"ho chi minh": "Asia/Ho_Chi_Minh",
			"hanoi":      "Asia/Ho_Chi_Minh",

			// Oceania
			"sydney":     "Australia/Sydney",
			"melbourne":  "Australia/Melbourne",
			"brisbane":   "Australia/Brisbane",
			"perth":      "Australia/Perth",
			"adelaide":   "Australia/Adelaide",
			"auckland":   "Pacific/Auckland",
			"wellington": "Pacific/Auckland",

			// South America
			"sao paulo":    "America/Sao_Paulo",
			"buenos aires": "America/Argentina/Buenos_Aires",
			"rio de janeiro": "America/Sao_Paulo",
			"lima":         "America/Lima",
			"bogota":       "America/Bogota",
			"santiago":     "America/Santiago",

			// Africa
			"cairo":        "Africa/Cairo",
			"johannesburg": "Africa/Johannesburg",
			"lagos":        "Africa/Lagos",
			"nairobi":      "Africa/Nairobi",
			"casablanca":   "Africa/Casablanca",
		},
		tzAliases: map[string]string{
			// Common timezone abbreviations
			"est":  "America/New_York",
			"edt":  "America/New_York",
			"cst":  "America/Chicago",
			"cdt":  "America/Chicago",
			"mst":  "America/Denver",
			"mdt":  "America/Denver",
			"pst":  "America/Los_Angeles",
			"pdt":  "America/Los_Angeles",
			"gmt":  "Europe/London",
			"utc":  "UTC",
			"bst":  "Europe/London",
			"cet":  "Europe/Paris",
			"cest": "Europe/Paris",
			"eet":  "Europe/Athens",
			"eest": "Europe/Athens",
			"ist":  "Asia/Kolkata",
			"jst":  "Asia/Tokyo",
			"kst":  "Asia/Seoul",
			"cst_china": "Asia/Shanghai",
			"hkt":  "Asia/Hong_Kong",
			"sgt":  "Asia/Singapore",
			"aest": "Australia/Sydney",
			"aedt": "Australia/Sydney",
			"awst": "Australia/Perth",
			"nzst": "Pacific/Auckland",
			"nzdt": "Pacific/Auckland",

			// Full timezone names
			"eastern":    "America/New_York",
			"central":    "America/Chicago",
			"mountain":   "America/Denver",
			"pacific":    "America/Los_Angeles",
			"london":     "Europe/London",
			"paris":      "Europe/Paris",
			"tokyo":      "Asia/Tokyo",
			"beijing":    "Asia/Shanghai",
			"india":      "Asia/Kolkata",
		},
	}
}

func (h *TimezoneHandler) Name() string {
	return "timezone"
}

func (h *TimezoneHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *TimezoneHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TimezoneHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Handle "time in {city}" patterns
	timeInPattern := regexp.MustCompile(`(?i)^(?:time\s+in|what\s+time\s+(?:is\s+it\s+)?in|current\s+time\s+in)\s+(.+?)\??$`)
	if matches := timeInPattern.FindStringSubmatch(query); len(matches) > 1 {
		return h.handleTimeIn(query, matches[1])
	}

	// Handle "{city} time" pattern
	cityTimePattern := regexp.MustCompile(`(?i)^(.+?)\s+time$`)
	if matches := cityTimePattern.FindStringSubmatch(query); len(matches) > 1 {
		city := strings.ToLower(matches[1])
		// Make sure it's not a generic "time" query
		if city != "current" && city != "local" && city != "what" {
			return h.handleTimeIn(query, matches[1])
		}
	}

	// Handle "{time} {tz1} to {tz2}" conversions
	convertPattern := regexp.MustCompile(`(?i)^(?:convert\s+)?(\d{1,2}(?::\d{2})?(?:\s*[ap]m)?)\s+([a-zA-Z_/]+)\s+(?:to|in)\s+([a-zA-Z_/]+)$`)
	if matches := convertPattern.FindStringSubmatch(query); len(matches) > 3 {
		return h.handleConversion(query, matches[1], matches[2], matches[3])
	}

	return nil, nil
}

func (h *TimezoneHandler) handleTimeIn(query, location string) (*Answer, error) {
	location = strings.TrimSpace(location)
	tzName := h.resolveTZ(location)

	if tzName == "" {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone",
			Content: fmt.Sprintf("Unknown location or timezone: %s", location),
		}, nil
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone",
			Content: fmt.Sprintf("Could not load timezone: %s", tzName),
		}, nil
	}

	now := time.Now().In(loc)
	localNow := time.Now()

	// Calculate offset from local time
	_, localOffset := localNow.Zone()
	_, remoteOffset := now.Zone()
	diffHours := float64(remoteOffset-localOffset) / 3600

	var offsetStr string
	if diffHours == 0 {
		offsetStr = "same time as you"
	} else if diffHours > 0 {
		offsetStr = fmt.Sprintf("%.1f hours ahead", diffHours)
	} else {
		offsetStr = fmt.Sprintf("%.1f hours behind", -diffHours)
	}

	// Get the timezone abbreviation
	zoneName, _ := now.Zone()

	return &Answer{
		Type:  AnswerTypeTime,
		Query: query,
		Title: fmt.Sprintf("Time in %s", strings.Title(location)),
		Content: fmt.Sprintf(`<div class="timezone-result">
<div class="time-display"><strong>%s</strong></div>
<div class="date-display">%s</div>
<div class="timezone-info">%s (%s)</div>
<div class="offset-info"><small>%s</small></div>
</div>`,
			now.Format("3:04 PM"),
			now.Format("Monday, January 2, 2006"),
			zoneName,
			tzName,
			offsetStr),
		Data: map[string]interface{}{
			"time":     now.Format(time.RFC3339),
			"timezone": tzName,
			"zone":     zoneName,
			"offset":   remoteOffset,
		},
	}, nil
}

func (h *TimezoneHandler) handleConversion(query, timeStr, fromTZ, toTZ string) (*Answer, error) {
	// Resolve timezone names
	fromTZName := h.resolveTZ(fromTZ)
	toTZName := h.resolveTZ(toTZ)

	if fromTZName == "" {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone Conversion",
			Content: fmt.Sprintf("Unknown source timezone: %s", fromTZ),
		}, nil
	}

	if toTZName == "" {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone Conversion",
			Content: fmt.Sprintf("Unknown target timezone: %s", toTZ),
		}, nil
	}

	fromLoc, err := time.LoadLocation(fromTZName)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone Conversion",
			Content: fmt.Sprintf("Could not load source timezone: %s", fromTZName),
		}, nil
	}

	toLoc, err := time.LoadLocation(toTZName)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone Conversion",
			Content: fmt.Sprintf("Could not load target timezone: %s", toTZName),
		}, nil
	}

	// Parse the input time
	parsedTime, err := h.parseTime(timeStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTime,
			Query:   query,
			Title:   "Time Zone Conversion",
			Content: fmt.Sprintf("Could not parse time: %s", timeStr),
		}, nil
	}

	// Create time in source timezone (using today's date)
	now := time.Now()
	sourceTime := time.Date(now.Year(), now.Month(), now.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, fromLoc)

	// Convert to target timezone
	targetTime := sourceTime.In(toLoc)

	// Check if day changed
	dayChange := ""
	if targetTime.Day() != sourceTime.Day() {
		if targetTime.After(sourceTime) {
			dayChange = " (next day)"
		} else {
			dayChange = " (previous day)"
		}
	}

	fromZone, _ := sourceTime.Zone()
	toZone, _ := targetTime.Zone()

	return &Answer{
		Type:  AnswerTypeTime,
		Query: query,
		Title: "Time Zone Conversion",
		Content: fmt.Sprintf(`<div class="timezone-conversion">
<div class="from-time"><strong>%s</strong> %s (%s)</div>
<div class="arrow">=</div>
<div class="to-time"><strong>%s</strong> %s (%s)%s</div>
</div>`,
			sourceTime.Format("3:04 PM"),
			fromZone,
			fromTZName,
			targetTime.Format("3:04 PM"),
			toZone,
			toTZName,
			dayChange),
		Data: map[string]interface{}{
			"fromTime":     sourceTime.Format(time.RFC3339),
			"toTime":       targetTime.Format(time.RFC3339),
			"fromTimezone": fromTZName,
			"toTimezone":   toTZName,
		},
	}, nil
}

func (h *TimezoneHandler) resolveTZ(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))

	// Check city mapping first
	if tz, ok := h.cityToTZ[input]; ok {
		return tz
	}

	// Check aliases
	if tz, ok := h.tzAliases[input]; ok {
		return tz
	}

	// Try as a direct IANA timezone name
	if strings.Contains(input, "/") {
		// Capitalize properly for IANA names
		parts := strings.Split(input, "/")
		for i, part := range parts {
			parts[i] = strings.Title(strings.ReplaceAll(part, "_", " "))
			parts[i] = strings.ReplaceAll(parts[i], " ", "_")
		}
		tzName := strings.Join(parts, "/")
		if _, err := time.LoadLocation(tzName); err == nil {
			return tzName
		}
	}

	// Try common variations
	variations := []string{
		"America/" + strings.Title(input),
		"Europe/" + strings.Title(input),
		"Asia/" + strings.Title(input),
		"Australia/" + strings.Title(input),
		"Pacific/" + strings.Title(input),
		"Africa/" + strings.Title(input),
	}

	for _, tz := range variations {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}

	return ""
}

func (h *TimezoneHandler) parseTime(timeStr string) (time.Time, error) {
	timeStr = strings.TrimSpace(strings.ToLower(timeStr))

	// Try various time formats
	formats := []string{
		"3:04pm",
		"3:04 pm",
		"3pm",
		"3 pm",
		"15:04",
		"15",
	}

	// Normalize the input
	timeStr = strings.ReplaceAll(timeStr, ".", ":")
	timeStr = strings.ReplaceAll(timeStr, "am", " am")
	timeStr = strings.ReplaceAll(timeStr, "pm", " pm")
	timeStr = strings.ReplaceAll(timeStr, "  ", " ")
	timeStr = strings.TrimSpace(timeStr)

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	// Try to parse just hours
	if hours, err := strconv.Atoi(timeStr); err == nil && hours >= 0 && hours <= 23 {
		return time.Date(0, 1, 1, hours, 0, 0, 0, time.UTC), nil
	}

	return time.Time{}, fmt.Errorf("could not parse time: %s", timeStr)
}
