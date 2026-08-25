// Package task models the 种蛋入孵任务聚合 (egg incubation task aggregate):
// the twelve business states, task generation, dual-person receipt, operation
// idempotency, and terminal protection coordinated within atomic transactions.
package task

import (
	"context"
	"time"
)

// TaskStatus enumerates the twelve strictly ordered task states. Transitions
// are driven by closed evidence and may not skip mandatory stages.
type TaskStatus string

const (
	StatusPendingLock       TaskStatus = "pending_lock"
	StatusPendingReceipt    TaskStatus = "pending_receipt"
	StatusResourcesOccupied TaskStatus = "resources_occupied"
	StatusCandling          TaskStatus = "candling"
	StatusSwabCulture       TaskStatus = "swab_culture"
	StatusRapidTest         TaskStatus = "rapid_test"
	StatusPhysicochemical   TaskStatus = "physicochemical"
	StatusPendingReview     TaskStatus = "pending_review"
	StatusAdmittable        TaskStatus = "admittable"
	StatusAdmitted          TaskStatus = "admitted"
	StatusIsolated          TaskStatus = "isolated"
	StatusCancelled         TaskStatus = "cancelled"
)

// transitions maps each state to the states it may legally advance into.
// Terminal states (admitted, isolated, cancelled) have no outgoing edges.
var transitions = map[TaskStatus][]TaskStatus{
	StatusPendingLock:       {StatusPendingReceipt, StatusCancelled},
	StatusPendingReceipt:    {StatusResourcesOccupied, StatusCancelled},
	StatusResourcesOccupied: {StatusCandling, StatusCancelled},
	StatusCandling:          {StatusSwabCulture, StatusCancelled},
	StatusSwabCulture:       {StatusRapidTest, StatusCancelled},
	StatusRapidTest:         {StatusPhysicochemical, StatusCancelled},
	StatusPhysicochemical:   {StatusPendingReview, StatusCancelled},
	StatusPendingReview:     {StatusAdmittable, StatusCancelled},
	StatusAdmittable:        {StatusAdmitted, StatusIsolated, StatusCancelled},
}

// Valid reports whether s is one of the twelve defined states.
func (s TaskStatus) Valid() bool {
	_, ok := transitions[s]
	if ok {
		return true
	}
	return s == StatusAdmitted || s == StatusIsolated || s == StatusCancelled
}

// Terminal reports whether s is a final state that rejects all further writes.
func (s TaskStatus) Terminal() bool {
	return s == StatusAdmitted || s == StatusIsolated || s == StatusCancelled
}

// CanTransitionTo reports whether advancing from s to next is legal.
func (s TaskStatus) CanTransitionTo(next TaskStatus) bool {
	if s.Terminal() {
		return false
	}
	for _, t := range transitions[s] {
		if t == next {
			return true
		}
	}
	return false
}

// IncubationTask is the aggregate root carrying batch identity, current
// generation, rule/fumigation snapshots, optimistic version and logical time.
type IncubationTask struct {
	ID               string
	BatchNo          string
	Status           TaskStatus
	Generation       int
	RuleSnapshot     int
	FumigationDigest string
	HouseID          string
	ShiftID          string
	Receivers        []string
	Version          int64
	LogicalTime      int64
	CreatedAt        time.Time
	FinalKind        string
	FinalVersion     int64
}

// Repository loads and saves tasks using the Version field for optimistic
// concurrency; a Save whose version no longer matches must fail atomically.
type Repository interface {
	Load(ctx context.Context, id string) (IncubationTask, error)
	Save(ctx context.Context, t IncubationTask) error
}
