package direct

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Real cron expression evaluation for the `cron:` direct answer.
//
// This file replaces a previous implementation that returned `now+1h … now+5h`
// regardless of the expression it was given — the field breakdown and
// human-readable description were real, but the "Next Runs" list was fabricated,
// which is exactly the "no stubs" violation AI.md PART 1 forbids. A cron page
// whose next-run times are wrong is worse than one that omits them, because the
// user has no way to tell.

// cronField is one parsed cron field: the set of values it matches, plus
// whether it was written as something other than a bare "*".
//
// The `restricted` flag is load-bearing. Vixie cron treats day-of-month and
// day-of-week differently depending on whether they were restricted: "0 0 1 * 1"
// means "the 1st OR any Monday", not "the 1st AND any Monday". Collapsing the
// two into a plain set loses that, so the distinction is kept from parse to match.
type cronField struct {
	set        map[int]bool
	restricted bool
}

func (f cronField) matches(v int) bool { return f.set[v] }

// any returns true when the field is a bare "*", which cron semantics treat as
// "unconstrained" for the day-of-month/day-of-week interaction.
func (f cronField) any() bool { return !f.restricted }

// cronSchedule is a fully parsed 5- or 6-field expression. An optional 7th
// year field is accepted and carried in `year`; expressions whose year field can
// never match again simply produce no next runs.
type cronSchedule struct {
	minute cronField
	hour   cronField
	dom    cronField
	month  cronField
	dow    cronField
	year   cronField
}

// cronMacros maps the @-style shorthands to their 5-field equivalents. These are
// part of the de-facto cron standard (Vixie cron, Cronie, cronie) and appear in
// real crontabs, so rejecting them as invalid is unhelpful rather than strict.
//
// The keys are lowercased; the caller normalizes before lookup.
var cronMacros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// cronMonthNames maps three-letter month names to their 1-based month number,
// so "0 0 * jan mon" works the way a crontab user expects.
var cronMonthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// cronDayNames maps three-letter day names to weekday numbers, Sunday = 0.
var cronDayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// parseCronSchedule parses a cron expression into a schedule.
//
// Accepts 5 fields (standard), 6 fields (with seconds as the leading field, as
// Quartz and several other schedulers use), 7 fields (with a trailing year),
// and the @-style macros. A 6-field expression is seconds-first, which is the
// opposite end from the year field — the two conventions cannot both be
// "the extra field", so the position decides.
// splitCronExpression normalizes any accepted syntax down to the canonical 5
// positional fields, plus the trailing year expression when one was present.
//
// Macros expand, a leading seconds field is dropped, and a 7th year field is
// peeled off. Keeping this separate from parseCronSchedule lets the handler show
// the user the same field breakdown the scheduler actually runs, rather than
// re-deriving field positions from the raw input and getting 6-field and
// 7-field forms wrong.
func splitCronExpression(expression string) (parts []string, yearExpr string, err error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return nil, "", fmt.Errorf("empty cron expression")
	}

	lower := strings.ToLower(expr)
	if strings.HasPrefix(lower, "@") {
		expansion, ok := cronMacros[lower]
		if !ok {
			return nil, "", fmt.Errorf("unknown cron macro %q", expr)
		}
		expr = expansion
	}

	parts = strings.Fields(expr)

	// Optional trailing year field (the Quartz 7-field convention).
	if len(parts) == 7 {
		yearExpr = parts[6]
		parts = parts[:6]
	}

	// Optional leading seconds field. The seconds value is validated but then
	// discarded: every run is reported at :00 of its minute, and a sub-minute
	// schedule has no meaning for "when does this next run".
	if len(parts) == 6 {
		if _, e := parseCronField(parts[0], 0, 59, nil); e != nil {
			return nil, "", fmt.Errorf("invalid seconds field %q: %w", parts[0], e)
		}
		parts = parts[1:]
	}

	if len(parts) != 5 {
		return nil, "", fmt.Errorf("expected 5-7 fields, got %d", len(parts))
	}
	return parts, yearExpr, nil
}

func parseCronSchedule(expression string) (*cronSchedule, error) {
	parts, yearExpr, err := splitCronExpression(expression)
	if err != nil {
		return nil, err
	}

	year := cronField{set: fullRange(1970, 2099)}
	if yearExpr != "" {
		f, e := parseCronField(yearExpr, 1970, 2099, nil)
		if e != nil {
			return nil, fmt.Errorf("invalid year field %q: %w", yearExpr, e)
		}
		year = f
	}

	specs := []struct {
		raw   string
		lo    int
		hi    int
		names map[string]int
	}{
		{parts[0], 0, 59, nil},
		{parts[1], 0, 23, nil},
		{parts[2], 1, 31, nil},
		{parts[3], 1, 12, cronMonthNames},
		{parts[4], 0, 6, cronDayNames},
	}

	fields := make([]cronField, 5)
	for i, spec := range specs {
		f, err := parseCronField(spec.raw, spec.lo, spec.hi, spec.names)
		if err != nil {
			return nil, fmt.Errorf("field %d (%q): %w", i+1, spec.raw, err)
		}
		fields[i] = f
	}

	return &cronSchedule{
		minute: fields[0],
		hour:   fields[1],
		dom:    fields[2],
		month:  fields[3],
		dow:    fields[4],
		year:   year,
	}, nil
}

func fullRange(lo, hi int) map[int]bool {
	m := make(map[int]bool, hi-lo+1)
	for v := lo; v <= hi; v++ {
		m[v] = true
	}
	return m
}

// parseCronField parses one comma-separated list of values, ranges, and steps.
//
// Supported forms, per field position:
//   - "*"        every value in range
//   - "5"        a single value
//   - "1-5"      an inclusive range
//   - "*/15"     a step over the whole range
//   - "1-20/5"   a step over a range
//   - "0,30"     a list of any of the above
func parseCronField(raw string, lo, hi int, names map[string]int) (cronField, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cronField{}, fmt.Errorf("empty field")
	}

	set := make(map[int]bool)
	restricted := raw != "*"

	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return cronField{}, fmt.Errorf("empty list element in %q", raw)
		}

		// Split off a step suffix, if any.
		step := 1
		rangePart := item
		if slash := strings.Index(item, "/"); slash >= 0 {
			rangePart = item[:slash]
			stepStr := item[slash+1:]
			n, err := strconv.Atoi(stepStr)
			if err != nil || n <= 0 {
				return cronField{}, fmt.Errorf("invalid step %q in %q", stepStr, item)
			}
			step = n
		}

		start, end := 0, 0
		switch {
		case rangePart == "*":
			start, end = lo, hi
		case strings.Contains(rangePart, "-"):
			bounds := strings.SplitN(rangePart, "-", 2)
			var err error
			if start, err = cronValue(bounds[0], names); err != nil {
				return cronField{}, fmt.Errorf("invalid range start %q: %w", bounds[0], err)
			}
			if end, err = cronValue(bounds[1], names); err != nil {
				return cronField{}, fmt.Errorf("invalid range end %q: %w", bounds[1], err)
			}
		default:
			v, err := cronValue(rangePart, names)
			if err != nil {
				return cronField{}, fmt.Errorf("invalid value %q: %w", rangePart, err)
			}
			// A step needs something to walk. "5/2" is not standard cron, so
			// read it as "from 5 to the end of the range, every 2" rather than
			// silently accepting a form whose meaning the user could not
			// predict.
			if step == 1 {
				start, end = v, v
			} else {
				start, end = v, hi
			}
		}

		if start < lo || end > hi {
			return cronField{}, fmt.Errorf("value %d-%d out of range %d-%d", start, end, lo, hi)
		}
		if start > end {
			return cronField{}, fmt.Errorf("range start %d is after end %d", start, end)
		}

		for v := start; v <= end; v += step {
			set[v] = true
		}
	}

	if len(set) == 0 {
		return cronField{}, fmt.Errorf("field %q matches nothing", raw)
	}
	return cronField{set: set, restricted: restricted}, nil
}

// cronValue parses one value token, accepting a month or day name when the field
// position allows it.
func cronValue(tok string, names map[string]int) (int, error) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return 0, fmt.Errorf("empty value")
	}
	if v, err := strconv.Atoi(tok); err == nil {
		return v, nil
	}
	if names != nil {
		if v, ok := names[strings.ToLower(tok)]; ok {
			return v, nil
		}
	}
	return 0, fmt.Errorf("not a number and not a known name")
}

// dayMatches reports whether t's date satisfies the dom/dow pair.
//
// When both fields are restricted, cron fires when *either* matches. Restricting
// both to mean "and" is a common misreading and produces schedules that fire far
// less often than the user intends.
func (s *cronSchedule) dayMatches(t time.Time) bool {
	switch {
	case s.dom.any() && s.dow.any():
		return true
	case s.dom.any():
		return s.dow.matches(int(t.Weekday()))
	case s.dow.any():
		return s.dom.matches(t.Day())
	default:
		return s.dom.matches(t.Day()) || s.dow.matches(int(t.Weekday()))
	}
}

// matches reports whether t falls on a scheduled run.
func (s *cronSchedule) matches(t time.Time) bool {
	return s.dayMatches(t) &&
		s.month.matches(int(t.Month())) &&
		s.year.matches(t.Year())
}

// maxCronSearchYears bounds the search for the next run. An expression that can
// never fire again — "0 0 1 1 * 2020", or "0 0 30 2 *" for a date that does not
// exist — would otherwise spin forever, so the walk gives up rather than hanging
// a request handler.
const maxCronSearchYears = 5

// nextRun returns the first scheduled time strictly after `after`, truncated to
// the minute, or ok=false if none is found within the search bound.
//
// The walk skips whole non-matching months, days, and hours rather than
// advancing a minute at a time, which is what lets a sparse expression like
// "0 0 29 2 *" resolve in milliseconds instead of scanning every minute of the
// next four years.
func (s *cronSchedule) nextRun(after time.Time) (time.Time, bool) {
	limitYear := after.Year() + maxCronSearchYears
	t := after.Truncate(time.Minute).Add(time.Minute)

	for t.Year() <= limitYear {
		if !s.year.matches(t.Year()) {
			t = time.Date(t.Year()+1, time.January, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.month.matches(int(t.Month())) {
			t = startOfNextMonth(t)
			continue
		}
		if !s.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
			continue
		}
		if !s.hour.matches(t.Hour()) {
			t = advanceHour(t)
			continue
		}
		if !s.minute.matches(t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		if s.matches(t) {
			return t, true
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, false
}

func startOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
}

// advanceHour moves t to the top of the next hour on the absolute timeline.
//
// It deliberately does not round the wall clock down with time.Date. On the day
// a zone falls back, 01:00 occurs twice, and time.Date normalises an ambiguous
// wall clock to the *earlier* instant. Reconstructing 01:00 and then adding an
// hour therefore returns to the same instant it started from: 01:00 EDT ->
// 01:00 EST -> 01:00 EDT -> ... An expression whose hour is 0 or 2 hit that loop
// and never returned, hanging the request handler for the length of the fall-back
// hour. Truncate works in absolute time, so the sequence is monotonic by
// construction, and the wall-clock hour is re-tested on the next pass.
func advanceHour(t time.Time) time.Time {
	return t.Truncate(time.Hour).Add(time.Hour)
}

// nextRuns returns up to count formatted run times after `after`.
//
// It returns fewer than count when the expression cannot fire again within the
// search bound. Returning a short list rather than padding it to length is
// deliberate: the behavior this replaced invented times to fill the quota, which
// is precisely the bug.
func (s *cronSchedule) nextRuns(after time.Time, count int) []string {
	if count <= 0 {
		return nil
	}
	runs := make([]string, 0, count)
	cursor := after
	for i := 0; i < count; i++ {
		next, ok := s.nextRun(cursor)
		if !ok {
			break
		}
		runs = append(runs, next.Format(time.RFC1123))
		cursor = next
	}
	return runs
}

// nextCronRuns parses an expression and computes its next run times. A parse
// failure is returned so the caller can render the invalid_cron answer instead
// of a fabricated list.
func nextCronRuns(expression string, after time.Time, count int) ([]string, error) {
	sched, err := parseCronSchedule(expression)
	if err != nil {
		return nil, err
	}
	return sched.nextRuns(after, count), nil
}
