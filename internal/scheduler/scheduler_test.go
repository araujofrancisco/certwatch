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
