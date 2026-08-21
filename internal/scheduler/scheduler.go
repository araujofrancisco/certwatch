package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CronExpr struct {
	Minute int
	Hour   int
	Day    int
	Month  int
	Weekday int
}

type Job struct {
	Name     string
	Expr     CronExpr
	Timezone *time.Location
	Handler  func(context.Context)
}

type Scheduler struct {
	mu   sync.Mutex
	jobs []*Job
}

func New() *Scheduler {
	return &Scheduler{}
}

func ParseCron(expr string) (CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return CronExpr{}, fmt.Errorf("invalid cron expression: %q (expected 5 fields)", expr)
	}
	min, err := parseField(fields[0], 0, 59)
	if err != nil {
		return CronExpr{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return CronExpr{}, fmt.Errorf("hour: %w", err)
	}
	day, err := parseField(fields[2], 1, 31)
	if err != nil {
		return CronExpr{}, fmt.Errorf("day: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return CronExpr{}, fmt.Errorf("month: %w", err)
	}
	weekday, err := parseWeekday(fields[4])
	if err != nil {
		return CronExpr{}, fmt.Errorf("weekday: %w", err)
	}
	return CronExpr{Minute: min, Hour: hour, Day: day, Month: month, Weekday: weekday}, nil
}

func parseField(field string, min, max int) (int, error) {
	if field == "*" {
		return -1, nil
	}
	v, err := strconv.Atoi(field)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", field)
	}
	if v < min || v > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", v, min, max)
	}
	return v, nil
}

func parseWeekday(field string) (int, error) {
	names := map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
		"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
		"thursday": 4, "friday": 5, "saturday": 6,
	}
	if v, ok := names[strings.ToLower(field)]; ok {
		return v, nil
	}
	return parseField(field, 0, 6)
}

func (e CronExpr) Matches(t time.Time) bool {
	if e.Minute >= 0 && t.Minute() != e.Minute {
		return false
	}
	if e.Hour >= 0 && t.Hour() != e.Hour {
		return false
	}
	if e.Month >= 0 && int(t.Month()) != e.Month {
		return false
	}
	// Standard cron semantics: when both day-of-month and day-of-week are
	// restricted, a match on either one selects the day.
	if e.Day >= 0 && e.Weekday >= 0 {
		return t.Day() == e.Day || int(t.Weekday()) == e.Weekday
	}
	if e.Day >= 0 && t.Day() != e.Day {
		return false
	}
	if e.Weekday >= 0 && int(t.Weekday()) != e.Weekday {
		return false
	}
	return true
}

// Next returns the first minute strictly after 'from' that matches the
// expression, evaluated in loc (UTC when nil). ok is false when no match
// exists within a 4-year search horizon.
func (e CronExpr) Next(from time.Time, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}
	t := from.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(4, 0, 0)

	for t.Before(limit) {
		// Month mismatch: nothing in this month can match.
		if e.Month >= 0 && int(t.Month()) != e.Month {
			t = dayStart(t).AddDate(0, 0, 1)
			continue
		}
		// Day fields. When both Day and Weekday are restricted, cron
		// semantics OR them: a day matching either constraint is selected.
		dayRestricted := e.Day >= 0
		weekdayRestricted := e.Weekday >= 0
		switch {
		case dayRestricted && weekdayRestricted:
			if t.Day() != e.Day && int(t.Weekday()) != e.Weekday {
				t = dayStart(t).AddDate(0, 0, 1)
				continue
			}
		case dayRestricted:
			if t.Day() != e.Day {
				t = dayStart(t).AddDate(0, 0, 1)
				continue
			}
		case weekdayRestricted:
			if int(t.Weekday()) != e.Weekday {
				t = dayStart(t).AddDate(0, 0, 1)
				continue
			}
		}
		// Hour.
		if e.Hour >= 0 && t.Hour() != e.Hour {
			if t.Hour() > e.Hour {
				t = dayStart(t).AddDate(0, 0, 1)
			} else {
				minute := 0
				if e.Minute >= 0 {
					minute = e.Minute
				}
				t = time.Date(t.Year(), t.Month(), t.Day(), e.Hour, minute, 0, 0, t.Location())
			}
			continue
		}
		// Minute.
		if e.Minute >= 0 && t.Minute() != e.Minute {
			hour := t.Hour()
			if t.Minute() > e.Minute {
				if e.Hour >= 0 {
					// Fixed hour already matched but its target minute has
					// passed (only possible via minute-stepping): move on.
					t = dayStart(t).AddDate(0, 0, 1)
					continue
				}
				hour++
			}
			t = time.Date(t.Year(), t.Month(), t.Day(), hour, e.Minute, 0, 0, t.Location())
			continue
		}
		// Day selection already applied above (with OR semantics); here we
		// only need to confirm month/hour/minute.
		if (e.Month < 0 || int(t.Month()) == e.Month) &&
			(e.Hour < 0 || t.Hour() == e.Hour) &&
			(e.Minute < 0 || t.Minute() == e.Minute) {
			return t, true
		}
		t = t.Add(time.Minute)
	}
	return limit, false
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (s *Scheduler) Add(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	jobs := make([]*Job, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.Unlock()
	for _, job := range jobs {
		go s.runJob(ctx, job)
	}
}

func (s *Scheduler) runJob(ctx context.Context, job *Job) {
	// Sleep until each job's next cron match instead of polling on a ticker.
	var lastRun time.Time

	for {
		next, ok := job.Expr.Next(time.Now(), job.Timezone)
		if !ok {
			// No future match within the search horizon; nothing to schedule.
			return
		}

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case fired := <-timer.C:
			// Guard against firing early (timer truncation, clock adjustments):
			// wait out any remaining gap before running.
			if delay := next.Sub(fired); delay > 0 {
				if !sleepCtx(ctx, delay) {
					return
				}
				fired = next
			}
			t := fired
			if job.Timezone != nil {
				t = t.In(job.Timezone)
			}
			if t.Truncate(time.Minute).Equal(lastRun.Truncate(time.Minute)) {
				continue
			}
			lastRun = t
			slog.Info("running scheduled job", "job", job.Name, "time", t.Format(time.RFC3339))
			runJob(job, ctx)
		}
	}
}

// runJob invokes a job handler with panic isolation: scheduled handlers run on
// bare goroutines with no HTTP recovery middleware, so a panicking handler
// would otherwise take down the whole process.
func runJob(job *Job, ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("scheduled job panicked", "job", job.Name, "panic", rec)
		}
	}()
	job.Handler(ctx)
}

// sleepCtx sleeps for d, returning false if ctx was cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
