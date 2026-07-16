package worker

import (
	"fmt"
	"sync"
	"testing"
)

func TestWorkerRuntimeRegistryBusyIdleAndCounters(t *testing.T) {
	registry := NewWorkerRuntimeRegistry()
	registry.RegisterWorker("worker-2")
	registry.RegisterWorker("worker-1")
	registry.MarkPoll("worker-1")
	registry.MarkBusy("worker-1", 42)
	registry.MarkSuccess("worker-1")
	registry.MarkIdle("worker-1")
	registry.MarkBusy("worker-1", 43)
	registry.MarkFailure("worker-1")
	registry.MarkIdle("worker-1")

	snapshot := registry.Snapshot()
	if len(snapshot.Workers) != 2 || snapshot.Workers[0].WorkerID != "worker-1" || snapshot.Workers[1].WorkerID != "worker-2" {
		t.Fatalf("snapshot is not sorted: %#v", snapshot.Workers)
	}
	worker := snapshot.Workers[0]
	if worker.Busy || worker.CurrentTaskID != 0 {
		t.Fatalf("worker was not marked idle: %#v", worker)
	}
	if worker.ProcessedCount != 2 || worker.SuccessCount != 1 || worker.FailureCount != 1 {
		t.Fatalf("unexpected counters: %#v", worker)
	}
	if worker.LastPollAt == nil || worker.LastClaimAt == nil || worker.LastSuccessAt == nil || worker.LastFailureAt == nil {
		t.Fatalf("runtime timestamps were not recorded: %#v", worker)
	}
}

func TestWorkerRuntimeRegistrySnapshotReturnsDeepCopy(t *testing.T) {
	registry := NewWorkerRuntimeRegistry()
	registry.RegisterWorker("worker-1")
	registry.MarkPoll("worker-1")

	first := registry.Snapshot()
	first.Workers[0].WorkerID = "changed"
	first.Workers[0].LastPollAt = nil

	second := registry.Snapshot()
	if second.Workers[0].WorkerID != "worker-1" || second.Workers[0].LastPollAt == nil {
		t.Fatalf("snapshot mutation leaked into registry: %#v", second.Workers[0])
	}
	originalPoll := *second.Workers[0].LastPollAt
	*second.Workers[0].LastPollAt = originalPoll.AddDate(1, 0, 0)
	third := registry.Snapshot()
	if third.Workers[0].LastPollAt.Equal(*second.Workers[0].LastPollAt) {
		t.Fatal("snapshot time pointer aliases registry state")
	}
}

func TestWorkerRuntimeRegistryConcurrentAccess(t *testing.T) {
	registry := NewWorkerRuntimeRegistry()
	const workerCount = 8
	const iterations = 100

	var wg sync.WaitGroup
	for index := 0; index < workerCount; index++ {
		workerID := fmt.Sprintf("worker-%02d", index)
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.RegisterWorker(workerID)
			for attempt := 0; attempt < iterations; attempt++ {
				registry.MarkPoll(workerID)
				registry.MarkBusy(workerID, uint64(attempt+1))
				registry.MarkSuccess(workerID)
				registry.MarkFailure(workerID)
				registry.MarkIdle(workerID)
				_ = registry.Snapshot()
			}
		}()
	}
	wg.Wait()

	snapshot := registry.Snapshot()
	if len(snapshot.Workers) != workerCount {
		t.Fatalf("registered workers = %d, want %d", len(snapshot.Workers), workerCount)
	}
	for _, worker := range snapshot.Workers {
		if worker.Busy || worker.ProcessedCount != iterations*2 ||
			worker.SuccessCount != iterations || worker.FailureCount != iterations {
			t.Fatalf("unexpected concurrent snapshot item: %#v", worker)
		}
	}
}
