package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/araujofrancisco/certwatch/internal/models"
)

// scanTask represents a request to scan a single domain.
type scanTask struct {
	domainID int64
	ctx      context.Context
	priority bool            // true for manual "Scan Now"; false for periodic/bulk
	result   chan scanTaskResult
}

type scanTaskResult struct {
	cert *models.Certificate
	err  error
}

// scanFunc is the function the queue calls to perform the actual domain scan.
type scanFunc func(ctx context.Context, domainID int64, timeout time.Duration) (*models.Certificate, error)

// scanQueue is an in-memory FIFO queue with a priority bypass and per-domain
// deduplication. Tasks are executed by a fixed pool of worker goroutines, gated
// by a semaphore that caps the number of concurrent scans. The queue and
// semaphore sizes are configurable via config/default.yaml
// (discovery.max_concurrent_scans and discovery.queue_size).
type scanQueue struct {
	highPri  chan scanTask
	lowPri   chan scanTask
	sem      chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	closed   bool
	active   map[int64]bool // domainIDs with a task in the queue or actively scanning
	scan     scanFunc
	timeout  time.Duration
}

// newScanQueue constructs a scan queue with the given concurrency, buffer
// size, and per-scan timeout. The returned queue starts its worker pool
// immediately.
func newScanQueue(maxConcurrent, queueSize int, timeout time.Duration, scan scanFunc) *scanQueue {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if queueSize < 1 {
		queueSize = 100
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	q := &scanQueue{
		highPri: make(chan scanTask, queueSize),
		lowPri:  make(chan scanTask, queueSize),
		sem:     make(chan struct{}, maxConcurrent),
		active:  make(map[int64]bool),
		timeout: timeout,
		scan:    scan,
	}
	q.startWorkers(maxConcurrent)
	return q
}

func (q *scanQueue) startWorkers(n int) {
	for i := 0; i < n; i++ {
		q.wg.Add(1)
		go q.worker()
	}
}

func (q *scanQueue) worker() {
	defer q.wg.Done()
	for {
		var task scanTask
		var ok bool

		// Block on high-priority first. Only fall through to the
		// low-priority channel when the high-priority one is empty.
		select {
		case task, ok = <-q.highPri:
			if !ok {
				return // channel closed, queue is stopping
			}
		default:
			select {
			case task, ok = <-q.highPri:
			case task, ok = <-q.lowPri:
			}
			if !ok {
				return // channel closed, queue is stopping
			}
		}

		// Acquire semaphore (or bail if context is cancelled).
		select {
		case q.sem <- struct{}{}:
		case <-task.ctx.Done():
			q.unmark(task.domainID)
			q.finish(task, nil, task.ctx.Err())
			continue
		}

		cert, err := q.scan(task.ctx, task.domainID, q.timeout)
		<-q.sem // release semaphore
		q.unmark(task.domainID)
		q.finish(task, cert, err)
	}
}

// unmark removes a domain from the active set (called when the task leaves the
// queue or finishes scanning).
func (q *scanQueue) unmark(domainID int64) {
	q.mu.Lock()
	delete(q.active, domainID)
	q.mu.Unlock()
}

func (q *scanQueue) finish(task scanTask, cert *models.Certificate, err error) {
	if task.result != nil {
		select {
		case task.result <- scanTaskResult{cert: cert, err: err}:
		case <-task.ctx.Done():
		}
	}
}

// Enqueue adds a task to the appropriate priority queue. If a scan for the
// same domain is already queued or running, the task is dropped (returns nil).
// It blocks (honoring the task's context) when the queue buffer is full,
// providing natural backpressure to callers.
func (q *scanQueue) Enqueue(task scanTask) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return fmt.Errorf("scan queue is stopped")
	}
	if q.active[task.domainID] {
		q.mu.Unlock()
		slog.Debug("scan already queued/running for domain, skipping", "domain_id", task.domainID)
		return nil
	}
	q.active[task.domainID] = true
	q.mu.Unlock()

	ch := q.lowPri
	if task.priority {
		ch = q.highPri
	}

	select {
	case ch <- task:
		return nil
	case <-task.ctx.Done():
		q.unmark(task.domainID)
		return task.ctx.Err()
	}
}

// EnqueueScan is a convenience wrapper for fire-and-forget enqueueing.
// It logs errors but does not block the caller beyond the queue buffer.
func (q *scanQueue) EnqueueScan(ctx context.Context, domainID int64, priority bool) {
	task := scanTask{
		domainID: domainID,
		ctx:      ctx,
		priority: priority,
	}
	if err := q.Enqueue(task); err != nil {
		slog.Error("failed to enqueue scan", "domain_id", domainID, "error", err)
	}
}

// Stop shuts down the queue. Workers finish in-flight scans immediately
// (honoring their context) but will not pick up new tasks. It blocks until
// all workers exit.
func (q *scanQueue) Stop() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()

	close(q.highPri)
	close(q.lowPri)
	q.wg.Wait()
}

// Pending returns the number of tasks waiting in the queues (not dispatched
// to a worker yet).
func (q *scanQueue) Pending() int {
	return len(q.highPri) + len(q.lowPri)
}

// InFlight returns the number of tasks currently being scanned.
func (q *scanQueue) InFlight() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.active)
}
