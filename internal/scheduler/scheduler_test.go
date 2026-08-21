package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseCron_AllWildcards(t *testing.T) {
	expr, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Minute != -1 || expr.Hour != -1 || expr.Day != -1 || expr.Month != -1 || expr.Weekday != -1 {
		t.Error("expected all wildcards to be -1")
	}
}

func TestParseCron_Specific(t *testing.T) {
	expr, err := ParseCron("30 8 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Minute != 30 || expr.Hour != 8 {
		t.Errorf("expected 30 8, got %d %d", expr.Minute, expr.Hour)
	}
}

func TestParseCron_WeekdayName(t *testing.T) {
	expr, err := ParseCron("0 9 * * MON")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Weekday != 1 {
		t.Errorf("expected Monday=1, got %d", expr.Weekday)
	}
}

func TestParseCron_WeekdayFullName(t *testing.T) {
	expr, err := ParseCron("0 9 * * Monday")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Weekday != 1 {
		t.Errorf("expected Monday=1, got %d", expr.Weekday)
	}
}

func TestParseCron_InvalidFields(t *testing.T) {
	tests := []string{
		"", "a b c d e", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * 32 * *", "* * * 0 *", "* * * 13 *",
	}
	for _, tc := range tests {
		_, err := ParseCron(tc)
		if err == nil {
			t.Errorf("expected error for %q", tc)
		}
	}
}

func TestParseCron_WrongFieldCount(t *testing.T) {
	_, err := ParseCron("* * * *")
	if err == nil {
		t.Error("expected error for 4-field cron")
	}
	_, err = ParseCron("* * * * * *")
	if err == nil {
		t.Error("expected error for 6-field cron")
	}
}

func TestCronExpr_Matches(t *testing.T) {
	expr, _ := ParseCron("30 8 15 6 1")
	now := time.Date(2026, 6, 15, 8, 30, 0, 0, time.UTC)
	if !expr.Matches(now) {
		t.Error("expected match")
	}
	if expr.Matches(now.Add(time.Hour)) {
		t.Error("expected no match for different hour")
	}
}

func TestCronExpr_WildcardMatch(t *testing.T) {
	expr, _ := ParseCron("* * * * *")
	now := time.Now()
	if !expr.Matches(now) {
		t.Error("wildcard should match any time")
	}
}

func TestCronExpr_Monthly(t *testing.T) {
	expr, _ := ParseCron("0 8 1 * *")
	t1 := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	if !expr.Matches(t1) {
		t.Error("expected match on 1st")
	}
	if expr.Matches(t2) {
		t.Error("expected no match on 2nd")
	}
}

func TestScheduler_Add(t *testing.T) {
	s := New()
	var count atomic.Int32
	s.Add(&Job{
		Name: "test",
		Expr: CronExpr{Minute: -1, Hour: -1, Day: -1, Month: -1, Weekday: -1},
		Handler: func(ctx context.Context) {
			count.Add(1)
		},
	})
	if len(s.jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(s.jobs))
	}
}

func TestCronExpr_Next(t *testing.T) {
	loc := time.UTC
	tests := []struct {
		name string
		expr string
		from time.Time
		want time.Time
	}{
		{
			name: "wildcard fires next minute",
			expr: "* * * * *",
			from: time.Date(2026, 8, 21, 10, 30, 15, 0, loc),
			want: time.Date(2026, 8, 21, 10, 31, 0, 0, loc),
		},
		{
			name: "daily at fixed time, same day ahead",
			expr: "30 8 * * *",
			from: time.Date(2026, 8, 21, 7, 0, 0, 0, loc),
			want: time.Date(2026, 8, 21, 8, 30, 0, 0, loc),
		},
		{
			name: "daily at fixed time, past this window",
			expr: "30 8 * * *",
			from: time.Date(2026, 8, 21, 8, 45, 0, 0, loc),
			want: time.Date(2026, 8, 22, 8, 30, 0, 0, loc),
		},
		{
			name: "wildcard hour fixed minute jumps forward",
			expr: "45 * * * *",
			from: time.Date(2026, 8, 21, 10, 50, 0, 0, loc),
			want: time.Date(2026, 8, 21, 11, 45, 0, 0, loc),
		},
		{
			name: "weekly skips days",
			expr: "0 9 * * Monday",
			// 2026-08-21 is a Friday.
			from: time.Date(2026, 8, 21, 12, 0, 0, 0, loc),
			want: time.Date(2026, 8, 24, 9, 0, 0, 0, loc),
		},
		{
			name: "monthly first day",
			expr: "0 8 1 * *",
			from: time.Date(2026, 8, 1, 9, 0, 0, 0, loc),
			want: time.Date(2026, 9, 1, 8, 0, 0, 0, loc),
		},
		{
			name: "day and weekday OR semantics",
			// Fires on the 1st OR on Mondays. 2026-08-24 is a Monday,
			// 2026-09-01 is a Tuesday.
			expr: "0 8 1 * 1",
			from: time.Date(2026, 8, 21, 0, 0, 0, 0, loc),
			want: time.Date(2026, 8, 24, 8, 0, 0, 0, loc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := ParseCron(tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := expr.Next(tc.from, loc)
			if !ok {
				t.Fatalf("expected match for %q", tc.expr)
			}
			if !got.Equal(tc.want) {
				t.Errorf("expr %q from %s: got %s, want %s", tc.expr, tc.from, got, tc.want)
			}
			if !expr.Matches(got) {
				t.Errorf("Next result %s does not match expr %q", got, tc.expr)
			}
			if !got.After(tc.from) {
				t.Errorf("Next must be strictly after 'from'")
			}
		})
	}
}

func TestScheduler_FiresOnSchedule(t *testing.T) {
	s := New()
	var count atomic.Int32

	// Fire every minute; wait long enough for the next minute boundary.
	s.Add(&Job{
		Name:     "every-minute",
		Expr:     CronExpr{Minute: -1, Hour: -1, Day: -1, Month: -1, Weekday: -1},
		Timezone: time.UTC,
		Handler: func(ctx context.Context) {
			count.Add(1)
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	// A wildcard job fires at the next minute boundary; wait until just
	// before the one after it so exactly one boundary is crossed regardless
	// of where inside the minute we started.
	nextBoundary := time.Now().Truncate(time.Minute).Add(95 * time.Second)
	time.Sleep(time.Until(nextBoundary))
	if count.Load() == 0 {
		t.Fatal("job did not fire at the minute boundary")
	}
	if count.Load() > 1 {
		t.Fatalf("job fired %d times, expected exactly 1", count.Load())
	}
}
