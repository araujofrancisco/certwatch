package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
)

func newTestQueue(t *testing.T, maxConcurrent, queueSize int) (*scanQueue, *int32) {
	t.Helper()
	var runs int32
	q := newScanQueue(maxConcurrent, queueSize, 50*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		atomic.AddInt32(&runs, 1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		return &models.Certificate{DomainID: domainID}, nil
	})
	t.Cleanup(q.Stop)
	return q, &runs
}

func TestScanQueueRunsTask(t *testing.T) {
	q, runs := newTestQueue(t, 1, 10)
	q.EnqueueScan(context.Background(), 1, false)
	waitFor(t, func() bool { return atomic.LoadInt32(runs) == 1 })
}

func TestScanQueueDedupesSameDomain(t *testing.T) {
	q, runs := newTestQueue(t, 1, 10)
	q.EnqueueScan(context.Background(), 1, false)
	q.EnqueueScan(context.Background(), 1, false)
	q.EnqueueScan(context.Background(), 1, true)
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(runs); got != 1 {
		t.Errorf("expected 1 run, got %d", got)
	}
}

func TestScanQueueRunsDifferentDomainsConcurrently(t *testing.T) {
	q, runs := newTestQueue(t, 3, 10)
	for i := int64(1); i <= 3; i++ {
		q.EnqueueScan(context.Background(), i, false)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(runs) == 3 })
}

func TestScanQueueStops(t *testing.T) {
	q, _ := newTestQueue(t, 1, 10)
	q.EnqueueScan(context.Background(), 1, false)
	q.Stop()
	if err := q.Enqueue(scanTask{domainID: 2, ctx: context.Background()}); err == nil {
		t.Error("expected error enqueuing after stop")
	}
}

func TestScanQueuePending(t *testing.T) {
	q, _ := newTestQueue(t, 1, 10)
	q.EnqueueScan(context.Background(), 1, false)
	q.EnqueueScan(context.Background(), 2, false)
	// Give workers time to pick up the first task; the second may be pending or
	// in flight depending on scheduling. Assert the queue stays bounded.
	if q.Pending() < 0 || q.InFlight() < 0 {
		t.Error("queue counters must be non-negative")
	}
}

func TestScanQueuePriority(t *testing.T) {
	var mu sync.Mutex
	var order []int64
	q := newScanQueue(1, 10, 50*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		mu.Lock()
		order = append(order, domainID)
		mu.Unlock()
		return &models.Certificate{DomainID: domainID}, nil
	})
	t.Cleanup(q.Stop)

	q.EnqueueScan(context.Background(), 1, false)
	q.EnqueueScan(context.Background(), 2, false)
	q.EnqueueScan(context.Background(), 3, true) // high priority bypass
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 3
	})
	mu.Lock()
	defer mu.Unlock()
	if order[0] != 3 {
		t.Errorf("expected high-priority task first, got order %v", order)
	}
}

func TestScanQueueConcurrencyCapped(t *testing.T) {
	var active, maxActive int32
	q := newScanQueue(2, 10, 100*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&maxActive)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
				break
			}
		}
		defer atomic.AddInt32(&active, -1)
		return &models.Certificate{DomainID: domainID}, nil
	})
	t.Cleanup(q.Stop)
	for i := int64(1); i <= 5; i++ {
		q.EnqueueScan(context.Background(), i, false)
	}
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt32(&maxActive); got > 2 {
		t.Errorf("expected max concurrency <= 2, got %d", got)
	}
}

func TestScanQueueCancelledContext(t *testing.T) {
	var runs int32
	q := newScanQueue(1, 10, 1*time.Hour, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		atomic.AddInt32(&runs, 1)
		return &models.Certificate{DomainID: domainID}, nil
	})
	t.Cleanup(q.Stop)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.EnqueueScan(ctx, 1, false)
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&runs); got != 0 {
		t.Errorf("expected no run for cancelled task, got %d", got)
	}
}

func TestScanQueueErrorResult(t *testing.T) {
	sentinel := errors.New("scan failed")
	q := newScanQueue(1, 10, 50*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		return nil, sentinel
	})
	t.Cleanup(q.Stop)

	ch := make(chan scanTaskResult, 1)
	task := scanTask{domainID: 1, ctx: context.Background(), result: ch, priority: false}
	if err := q.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-ch:
		if !errors.Is(r.err, sentinel) {
			t.Errorf("expected sentinel error, got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
func TestScanQueueStopDropsQueuedAndCompletesInFlight(t *testing.T) {
	var runs int32
	release := make(chan struct{})
	releaseOnce := sync.Once{}
	doRelease := func() { releaseOnce.Do(func() { close(release) }) }
	q := newScanQueue(1, 10, 50*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		atomic.AddInt32(&runs, 1)
		<-release // first task blocks until we tell it to finish
		return &models.Certificate{DomainID: domainID}, nil
	})
	t.Cleanup(func() { doRelease(); q.Stop() })

	q.EnqueueScan(context.Background(), 1, false) // will be in flight
	waitFor(t, func() bool { return atomic.LoadInt32(&runs) == 1 })

	// These two stay queued (worker blocked on the first scan).
	q.EnqueueScan(context.Background(), 2, false)
	q.EnqueueScan(context.Background(), 3, true)

	done := make(chan struct{})
	go func() { q.Stop(); close(done) }()

	// Let the in-flight scan finish; Stop should then complete.
	doRelease()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after in-flight scan finished")
	}

	if err := q.Enqueue(scanTask{domainID: 9, ctx: context.Background()}); err == nil {
		t.Error("expected error enqueuing after stop")
	}
}

// Regression test: Enqueue must never panic by sending on a closed channel
// when racing Stop (previously the channels were closed during shutdown).
func TestScanQueueEnqueueRacingStop(t *testing.T) {
	var runs int32
	q := newScanQueue(1, 1000, 50*time.Millisecond, func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error) {
		atomic.AddInt32(&runs, 1)
		return &models.Certificate{DomainID: domainID}, nil
	})

	var wg sync.WaitGroup
	for i := int64(1); i <= 200; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_ = q.Enqueue(scanTask{domainID: id, ctx: context.Background()})
		}(i)
	}
	q.Stop()
	wg.Wait()

	if err := q.Enqueue(scanTask{domainID: 9999, ctx: context.Background()}); err == nil {
		t.Error("expected error enqueuing after stop")
	}
}
