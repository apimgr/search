package direct

import (
	"testing"
	"time"

	// Embeds the zoneinfo database so the DST tests behave the same on a host
	// with no system tzdata. Without it, LoadLocation returns an error on a bare
	// container image and the fall-back test silently degrades to a no-op.
	_ "time/tzdata"
)

// cronRef is the fixed "now" every schedule test evaluates from: Wednesday
// 2026-09-30 12:34:56 UTC. A fixed instant is what makes these assertions
// meaningful — the implementation this replaced returned now+1h through now+5h,
// which is always "in the future" and would have passed a looseness test.
var cronRef = time.Date(2026, 9, 30, 12, 34, 56, 0, time.UTC)

// TestNextRunIsCorrect is the core claim: the first run is the one a real cron
// daemon would fire, not merely some future time.
func TestNextRunIsCorrect(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"every five minutes", "*/5 * * * *", "2026-09-30 12:35:00"},
		{"next matching minute in the same hour", "*/20 * * * *", "2026-09-30 12:40:00"},
		{"hourly on the hour", "@hourly", "2026-09-30 13:00:00"},
		{"hour already passed today rolls to tomorrow", "0 9 * * *", "2026-10-01 09:00:00"},
		{"on the hour", "0 * * * *", "2026-09-30 13:00:00"},
		{"daily at midnight", "@daily", "2026-10-01 00:00:00"},
		{"weekly on sunday", "@weekly", "2026-10-04 00:00:00"},
		{"monthly on the first", "@monthly", "2026-10-01 00:00:00"},
		{"yearly", "@yearly", "2027-01-01 00:00:00"},
		{"weekdays only skips the weekend", "0 9 * * 1-5", "2026-10-01 09:00:00"},
		{"sunday at 14:30", "30 14 * * 0", "2026-10-04 14:30:00"},
		{"explicit day of month", "0 0 1 * *", "2026-10-01 00:00:00"},
		{"explicit month and day", "0 0 1 1 *", "2027-01-01 00:00:00"},
		{"leap day skips the non-leap year", "0 0 29 2 *", "2028-02-29 00:00:00"},
		{"range with a step over hours", "15,45 8-17/4 * * *", "2026-09-30 12:45:00"},
		{"month name", "0 0 1 jan *", "2027-01-01 00:00:00"},
		{"day name", "0 0 * * sun", "2026-10-04 00:00:00"},
		{"seconds first is dropped", "30 0 9 * * *", "2026-10-01 09:00:00"},
		// A trailing year needs the full seven-field form, because a six-field
		// expression is always read as seconds-first.
		{"trailing year field is honoured", "0 0 0 1 1 * 2028", "2028-01-01 00:00:00"},

		// Both day fields restricted means "either", per Vixie cron. Reading it
		// as "and" makes this schedule fire once every few years instead of
		// twice a month, which is the classic way a cron explainer goes wrong.
		{"day of month OR day of week", "0 0 13 * 5", "2026-10-02 00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := parseCronSchedule(tt.expr)
			if err != nil {
				t.Fatalf("parseCronSchedule(%q) error = %v", tt.expr, err)
			}
			got, ok := sched.nextRun(cronRef)
			if !ok {
				t.Fatalf("nextRun(%q) found no run, want %s", tt.expr, tt.want)
			}
			if layout := got.Format("2006-01-02 15:04:05"); layout != tt.want {
				t.Errorf("nextRun(%q) = %s, want %s", tt.expr, layout, tt.want)
			}
		})
	}
}

// TestDayOfMonthOrDayOfWeek pins the "either when both restricted" rule
// explicitly, by checking the two dates the rule distinguishes.
func TestDayOfMonthOrDayOfWeek(t *testing.T) {
	sched, err := parseCronSchedule("0 0 13 * 5")
	if err != nil {
		t.Fatalf("parseCronSchedule() error = %v", err)
	}

	// Friday 2026-10-02 is not the 13th, but dow=5 matches.
	friday := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if !sched.dayMatches(friday) {
		t.Error("a matching day-of-week was rejected; both-restricted must mean OR")
	}

	// The 13th 2026-10-13 is a Tuesday, not a Friday, but dom=13 matches.
	thirteenth := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	if !sched.dayMatches(thirteenth) {
		t.Error("a matching day-of-month was rejected; both-restricted must mean OR")
	}

	// Saturday 2026-10-10 matches neither and must not fire.
	saturday := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if sched.dayMatches(saturday) {
		t.Error("a date matching neither field was accepted")
	}

	// With only day-of-week restricted, day-of-month is ignored entirely: a
	// Friday fires whatever its day of month, and only Fridays fire.
	weekly, err := parseCronSchedule("0 0 * * 5")
	if err != nil {
		t.Fatalf("parseCronSchedule() error = %v", err)
	}
	if !weekly.dayMatches(friday) {
		t.Error("day-of-month must be unconstrained when it is written as *")
	}
	if weekly.dayMatches(saturday) {
		t.Error("a Saturday was accepted with dow=5")
	}
}

// TestNextRunsStopsWhenTheScheduleCannotFire covers the two ways an expression
// can be syntactically valid and yet never fire. The behavior this replaced
// padded the list to the requested length with invented times; returning short
// is the honest result.
func TestNextRunsStopsWhenTheScheduleCannotFire(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"a day that does not exist", "0 0 30 2 *"},
		{"a year already in the past", "0 0 1 1 * 2020"},
		{"a month that does not exist", "0 0 1 13 *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs, err := nextCronRuns(tt.expr, cronRef, 5)
			if err != nil {
				// A parse rejection is also an acceptable answer; what is not
				// acceptable is inventing run times.
				return
			}
			if len(runs) != 0 {
				t.Errorf("nextCronRuns(%q) = %v, want no runs", tt.expr, runs)
			}
		})
	}
}

// TestNextRunsIsStrictlyIncreasingAndInTheFuture guards the sequence itself
// rather than any single value: duplicates or times in the past would make a
// correct-looking first entry useless.
func TestNextRunsIsStrictlyIncreasingAndInTheFuture(t *testing.T) {
	exprs := []string{
		"* * * * *", "*/5 * * * *", "0 9 * * 1-5", "0 0 1 * *",
		"30 14 * * *", "30 14 * * 0", "@daily", "@hourly", "0 0 13 * 5", "0 0 29 2 *",
	}

	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			runs, err := nextCronRuns(expr, cronRef, 5)
			if err != nil {
				t.Fatalf("nextCronRuns() error = %v", err)
			}
			if len(runs) != 5 {
				t.Fatalf("got %d runs, want 5: %v", len(runs), runs)
			}
			prev := cronRef
			for i, run := range runs {
				parsed, err := time.Parse(time.RFC1123, run)
				if err != nil {
					t.Fatalf("run %d (%q) is not RFC1123: %v", i, run, err)
				}
				if !parsed.After(prev) {
					t.Errorf("run %d (%s) is not after the previous run (%s)", i, parsed, prev)
				}
				prev = parsed
			}
		})
	}
}

// TestNextRunIsExactlyOneMinuteAfterTheCurrentMinute covers the boundary. A
// schedule that already matches the current minute must not report that minute
// as "next" — the run in progress is not a future run.
func TestNextRunIsExactlyOneMinuteAfterTheCurrentMinute(t *testing.T) {
	sched, err := parseCronSchedule("* * * * *")
	if err != nil {
		t.Fatalf("parseCronSchedule() error = %v", err)
	}

	exact := time.Date(2026, 9, 30, 12, 34, 0, 0, time.UTC)
	got, ok := sched.nextRun(exact)
	if !ok {
		t.Fatal("nextRun() found no run for a per-minute schedule")
	}
	if want := exact.Add(time.Minute); !got.Equal(want) {
		t.Errorf("nextRun(%s) = %s, want %s", exact, got, want)
	}
}

// TestParseCronFieldCoversTheStandardForms exercises each syntax a crontab user
// can write, since a form silently parsing to the wrong set is indistinguishable
// from one that does not parse at all.
func TestParseCronFieldCoversTheStandardForms(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		lo    int
		hi    int
		want  []int
		errIs bool
	}{
		{name: "wildcard", raw: "*", lo: 0, hi: 5, want: []int{0, 1, 2, 3, 4, 5}},
		{name: "single value", raw: "3", lo: 0, hi: 5, want: []int{3}},
		{name: "inclusive range", raw: "1-4", lo: 0, hi: 5, want: []int{1, 2, 3, 4}},
		{name: "step over range", raw: "*/2", lo: 0, hi: 5, want: []int{0, 2, 4}},
		{name: "step over subrange", raw: "1-5/2", lo: 0, hi: 6, want: []int{1, 3, 5}},
		{name: "list", raw: "0,2,4", lo: 0, hi: 5, want: []int{0, 2, 4}},
		{name: "mixed list and range", raw: "1-2,9", lo: 0, hi: 9, want: []int{1, 2, 9}},
		{name: "step from a value", raw: "2/3", lo: 0, hi: 7, want: []int{2, 5}},
		{name: "surrounding spaces tolerated", raw: " 1 , 3 ", lo: 0, hi: 5, want: []int{1, 3}},

		{name: "value below range", raw: "9", lo: 0, hi: 5, errIs: true},
		{name: "range start above end", raw: "5-1", lo: 0, hi: 9, errIs: true},
		{name: "zero step", raw: "*/0", lo: 0, hi: 5, errIs: true},
		{name: "negative step", raw: "*/-2", lo: 0, hi: 5, errIs: true},
		{name: "non-numeric step", raw: "*/x", lo: 0, hi: 5, errIs: true},
		{name: "empty field", raw: "", lo: 0, hi: 5, errIs: true},
		{name: "empty list element", raw: "1,,2", lo: 0, hi: 5, errIs: true},
		{name: "garbage", raw: "abc", lo: 0, hi: 5, errIs: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, err := parseCronField(tt.raw, tt.lo, tt.hi, nil)
			if tt.errIs {
				if err == nil {
					t.Fatalf("parseCronField(%q) succeeded, want an error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCronField(%q) error = %v", tt.raw, err)
			}
			if len(field.set) != len(tt.want) {
				t.Errorf("parseCronField(%q) matched %d values %v, want %d: %v",
					tt.raw, len(field.set), field.set, len(tt.want), tt.want)
			}
			for _, v := range tt.want {
				if !field.matches(v) {
					t.Errorf("parseCronField(%q) does not match %d", tt.raw, v)
				}
			}
		})
	}
}

// TestParseCronFieldNameAliases covers the three-letter month and day names that
// appear in real crontabs.
func TestParseCronFieldNameAliases(t *testing.T) {
	months, err := parseCronField("jan,jul,dec", 1, 12, cronMonthNames)
	if err != nil {
		t.Fatalf("month names error = %v", err)
	}
	for _, v := range []int{1, 7, 12} {
		if !months.matches(v) {
			t.Errorf("month field does not match %d", v)
		}
	}

	days, err := parseCronField("sun,sat", 0, 6, cronDayNames)
	if err != nil {
		t.Fatalf("day names error = %v", err)
	}
	for _, v := range []int{0, 6} {
		if !days.matches(v) {
			t.Errorf("day field does not match %d", v)
		}
	}

	// Names are not valid where the position does not allow them.
	if _, err := parseCronField("jan", 0, 59, nil); err == nil {
		t.Error("a month name was accepted in the minute field")
	}
}

// TestSplitCronExpression pins the field normalization. The handler renders this
// breakdown, so a mislabeled field is user-visible even when the schedule itself
// resolves correctly.
func TestSplitCronExpression(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		wantParts []string
		wantYear  string
		errIs     bool
	}{
		{
			name:      "standard five fields",
			expr:      "0 9 * * 1-5",
			wantParts: []string{"0", "9", "*", "*", "1-5"},
		},
		{
			name:      "macro expands to five fields",
			expr:      "@daily",
			wantParts: []string{"0", "0", "*", "*", "*"},
		},
		{
			name:      "macro is case insensitive",
			expr:      "@Weekly",
			wantParts: []string{"0", "0", "*", "*", "0"},
		},
		{
			name:      "leading seconds field is dropped",
			expr:      "45 0 9 * * *",
			wantParts: []string{"0", "9", "*", "*", "*"},
		},
		{
			name:      "trailing year field is peeled off",
			expr:      "0 0 0 1 1 * 2030",
			wantParts: []string{"0", "0", "1", "1", "*"},
			wantYear:  "2030",
		},
		{
			name:      "surrounding whitespace tolerated",
			expr:      "  0   9  *  *  *  ",
			wantParts: []string{"0", "9", "*", "*", "*"},
		},

		{name: "empty expression", expr: "", errIs: true},
		{name: "too few fields", expr: "0 9 * *", errIs: true},
		{name: "too many fields", expr: "0 0 9 * * * * *", errIs: true},
		{name: "unknown macro", expr: "@fortnightly", errIs: true},
		{name: "bare at sign", expr: "@", errIs: true},
		{name: "bad seconds field", expr: "99 0 9 * * *", errIs: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, year, err := splitCronExpression(tt.expr)
			if tt.errIs {
				if err == nil {
					t.Fatalf("splitCronExpression(%q) succeeded, want an error", tt.expr)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitCronExpression(%q) error = %v", tt.expr, err)
			}
			if len(parts) != len(tt.wantParts) {
				t.Fatalf("splitCronExpression(%q) = %v, want %v", tt.expr, parts, tt.wantParts)
			}
			for i := range parts {
				if parts[i] != tt.wantParts[i] {
					t.Errorf("splitCronExpression(%q) field %d = %q, want %q",
						tt.expr, i+1, parts[i], tt.wantParts[i])
				}
			}
			if year != tt.wantYear {
				t.Errorf("splitCronExpression(%q) year = %q, want %q", tt.expr, year, tt.wantYear)
			}
		})
	}
}

// TestParseCronScheduleRejectsBadExpressions covers the whole-expression errors,
// which are what turn a mistyped query into the invalid_cron answer.
func TestParseCronScheduleRejectsBadExpressions(t *testing.T) {
	tests := []struct {
		reason string
		expr   string
	}{
		{"empty", ""},
		{"prose", "not a cron expression"},
		{"minute above 59", "60 * * * *"},
		{"hour above 23", "* 24 * * *"},
		{"day above 31", "* * 32 * *"},
		{"month above 12", "* * * 13 *"},
		{"day of week above 6", "* * * * 7"},
		{"zero step", "*/0 * * * *"},
		{"year out of range", "0 0 1 1 * 1960"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if sched, err := parseCronSchedule(tt.expr); err == nil {
				t.Errorf("parseCronSchedule(%q) unexpectedly succeeded: %+v", tt.expr, sched)
			}
		})
	}
}

// TestNextRunTerminatesAcrossDSTTransitions is the regression test for the
// infinite loop this engine shipped with.
//
// On the day a zone falls back, wall-clock 01:00 happens twice. Reconstructing
// the hour with time.Date normalizes the ambiguous time to the earlier instant,
// so adding an hour from 01:00 EST lands back on 01:00 EST forever. Every
// expression whose hour field excludes the repeated hour — "0 0 1 * *",
// "0 0 1 * *", "0 14 * * *" — spun the request handler for the length of that hour. The
// search must still terminate, and every run it finds must be after the cursor.
func TestNextRunTerminatesAcrossDSTTransitions(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	// The autumn transition is 2026-11-01: 01:00-02:00 EDT repeats as 01:00 EST.
	// The spring transition is 2027-03-14: 02:00-03:00 EST does not exist.
	cases := []struct {
		name  string
		start time.Time
	}{
		{"before the fall back", time.Date(2026, 10, 31, 12, 0, 0, 0, ny)},
		{"inside the repeated hour", time.Date(2026, 11, 1, 1, 30, 0, 0, ny)},
		{"before the spring forward", time.Date(2027, 3, 13, 12, 0, 0, 0, ny)},
		{"in the skipped hour", time.Date(2027, 3, 14, 2, 30, 0, 0, ny)},
	}

	exprs := []string{
		"0 0 1 * *", "0 14 * * *", "30 14 * * 0", "@daily", "@hourly", "0 0 1 1 *", "*/5 * * * *",
	}

	for _, c := range cases {
		for _, expr := range exprs {
			t.Run(c.name+"/"+expr, func(t *testing.T) {
				done := make(chan []string, 1)
				go func() {
					runs, err := nextCronRuns(expr, c.start, 5)
					if err != nil {
						done <- nil
						return
					}
					done <- runs
				}()

				select {
				case runs := <-done:
					for i, run := range runs {
						parsed, perr := time.Parse(time.RFC1123, run)
						if perr != nil {
							t.Fatalf("run %d (%q) is not RFC1123: %v", i, run, perr)
						}
						if !parsed.After(c.start) {
							t.Errorf("run %d (%s) is not after the start (%s)", i, parsed, c.start)
						}
					}
				case <-time.After(10 * time.Second):
					t.Fatal("nextCronRuns did not terminate; the search is looping")
				}
			})
		}
	}
}

// TestNextRunIsFastForSparseSchedules guards the request-latency budget. A
// per-minute walk over a 5-year span is about 2.6M iterations, which would turn
// every "0 0 29 2 *" query into a slow response; skipping whole non-matching
// months keeps it instant.
func TestNextRunIsFastForSparseSchedules(t *testing.T) {
	exprs := []string{
		"0 0 29 2 *", "0 0 1 1 *", "59 23 31 12 *", "0 0 0 1 1 * 2099", "0 0 30 2 *",
	}

	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				nextCronRuns(expr, cronRef, 5) //nolint:errcheck // the error path has its own test
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("computing next runs took more than 5s")
			}
		})
	}
}

// TestNextRunFollowsItsOwnOutput checks that evaluating from a cursor the caller
// computed agrees with the sequence nextRuns returns. nextRuns feeds each result
// back in as the next cursor, so a schedule that drifted under that feedback
// would show up as an inconsistent single-step result.
func TestNextRunFollowsItsOwnOutput(t *testing.T) {
	expr := "0 0 13 * 5"
	sched, err := parseCronSchedule(expr)
	if err != nil {
		t.Fatalf("parseCronSchedule() error = %v", err)
	}

	runs, err := nextCronRuns(expr, cronRef, 5)
	if err != nil {
		t.Fatalf("nextCronRuns() error = %v", err)
	}

	cursor := cronRef
	for i, run := range runs {
		want, ok := sched.nextRun(cursor)
		if !ok {
			t.Fatalf("run %d: no run after %s, but the list contained %q", i, cursor, run)
		}
		got, _ := time.Parse(time.RFC1123, run)
		if !got.Equal(want) {
			t.Errorf("run %d = %s, want %s", i, got, want)
		}
		cursor = want
	}
}
