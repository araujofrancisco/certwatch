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
	if e.Day >= 0 && t.Day() != e.Day {
		return false
	}
	if e.Month >= 0 && int(t.Month()) != e.Month {
		return false
	}
	if e.Weekday >= 0 && int(t.Weekday()) != e.Weekday {
		return false
	}
	return true
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
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	lastRun := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if job.Timezone != nil {
				t = t.In(job.Timezone)
			}
			if job.Expr.Matches(t) {
				last := lastRun[job.Name]
				if t.Truncate(time.Minute).Equal(last.Truncate(time.Minute)) {
					continue
				}
				lastRun[job.Name] = t
				slog.Info("running scheduled job", "job", job.Name, "time", t.Format(time.RFC3339))
				job.Handler(ctx)
			}
		}
	}
}
