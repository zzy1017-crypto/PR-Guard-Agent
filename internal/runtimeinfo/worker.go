package runtimeinfo

import "time"

type WorkerRuntime struct {
	WorkerID string

	Busy          bool
	CurrentTaskID uint64

	LastPollAt    *time.Time
	LastClaimAt   *time.Time
	LastSuccessAt *time.Time
	LastFailureAt *time.Time

	ProcessedCount uint64
	SuccessCount   uint64
	FailureCount   uint64
}

type WorkerRuntimeSnapshot struct {
	StartedAt time.Time
	Workers   []WorkerRuntime
}
