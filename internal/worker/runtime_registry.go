package worker

import (
	"sort"
	"sync"
	"time"

	"pr-guard-agent/internal/runtimeinfo"
)

type WorkerRuntime = runtimeinfo.WorkerRuntime
type WorkerRuntimeSnapshot = runtimeinfo.WorkerRuntimeSnapshot

type WorkerRuntimeRegistry struct {
	mu        sync.RWMutex
	workers   map[string]*WorkerRuntime
	startedAt time.Time
}

func NewWorkerRuntimeRegistry() *WorkerRuntimeRegistry {
	return &WorkerRuntimeRegistry{
		workers:   make(map[string]*WorkerRuntime),
		startedAt: time.Now(),
	}
}

func (r *WorkerRuntimeRegistry) RegisterWorker(workerID string) {
	if r == nil || workerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[workerID]; exists {
		return
	}
	r.workers[workerID] = &WorkerRuntime{WorkerID: workerID}
}

func (r *WorkerRuntimeRegistry) MarkPoll(workerID string) {
	r.update(workerID, func(worker *WorkerRuntime, now time.Time) {
		worker.LastPollAt = timePointer(now)
	})
}

func (r *WorkerRuntimeRegistry) MarkBusy(workerID string, taskID uint64) {
	r.update(workerID, func(worker *WorkerRuntime, now time.Time) {
		worker.Busy = true
		worker.CurrentTaskID = taskID
		worker.LastClaimAt = timePointer(now)
	})
}

func (r *WorkerRuntimeRegistry) MarkSuccess(workerID string) {
	r.update(workerID, func(worker *WorkerRuntime, now time.Time) {
		worker.LastSuccessAt = timePointer(now)
		worker.ProcessedCount++
		worker.SuccessCount++
	})
}

func (r *WorkerRuntimeRegistry) MarkFailure(workerID string) {
	r.update(workerID, func(worker *WorkerRuntime, now time.Time) {
		worker.LastFailureAt = timePointer(now)
		worker.ProcessedCount++
		worker.FailureCount++
	})
}

func (r *WorkerRuntimeRegistry) MarkIdle(workerID string) {
	r.update(workerID, func(worker *WorkerRuntime, _ time.Time) {
		worker.Busy = false
		worker.CurrentTaskID = 0
	})
}

func (r *WorkerRuntimeRegistry) Snapshot() WorkerRuntimeSnapshot {
	if r == nil {
		return WorkerRuntimeSnapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := WorkerRuntimeSnapshot{
		StartedAt: r.startedAt,
		Workers:   make([]WorkerRuntime, 0, len(r.workers)),
	}
	for _, worker := range r.workers {
		copy := *worker
		copy.LastPollAt = cloneTime(worker.LastPollAt)
		copy.LastClaimAt = cloneTime(worker.LastClaimAt)
		copy.LastSuccessAt = cloneTime(worker.LastSuccessAt)
		copy.LastFailureAt = cloneTime(worker.LastFailureAt)
		snapshot.Workers = append(snapshot.Workers, copy)
	}
	sort.Slice(snapshot.Workers, func(i, j int) bool {
		return snapshot.Workers[i].WorkerID < snapshot.Workers[j].WorkerID
	})
	return snapshot
}

func (r *WorkerRuntimeRegistry) update(workerID string, update func(*WorkerRuntime, time.Time)) {
	if r == nil || workerID == "" || update == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	worker, exists := r.workers[workerID]
	if !exists {
		worker = &WorkerRuntime{WorkerID: workerID}
		r.workers[workerID] = worker
	}
	update(worker, time.Now())
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
