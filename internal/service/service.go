// Package service implements the application orchestration: each command runs
// as a single task transaction spanning catalog validation, idempotency
// registration, lease changes, coverage writes, evidence appends, state
// advancement and audit, so any failure rolls the whole batch back.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// Stable service errors surfaced as business codes by the API layer.
var (
	ErrTaskNotFound      = errors.New("service: task not found")
	ErrTerminal          = errors.New("service: task is terminal")
	ErrStaleGeneration   = errors.New("service: stale task generation")
	ErrInvalidState      = errors.New("service: invalid task state for command")
	ErrOperationConflict = errors.New("service: operation id conflict")
	ErrNotQualified      = errors.New("service: person not qualified")
)

// Service coordinates commands and queries over the persistent store.
type Service struct {
	store   *store.Store
	devices map[string]evidence.DevicePort
}

// New builds a service with the default healthy device registry.
func New(s *store.Store) *Service {
	return &Service{store: s, devices: defaultDevices()}
}

// Store exposes the backing store for query handlers.
func (s *Service) Store() *store.Store { return s.store }

// SetDevice overrides the device port for a device id (used by tests to script
// failures). It returns the service for fluent wiring.
func (s *Service) SetDevice(id string, p evidence.DevicePort) *Service {
	s.devices[id] = p
	return s
}

// CommandResult is the uniform command response body.
type CommandResult struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Generation  int    `json:"generation"`
	Version     int64  `json:"version"`
	LogicalTime int64  `json:"logical_time"`
}

func newResult(t task.IncubationTask) CommandResult {
	return CommandResult{
		TaskID:      t.ID,
		Status:      string(t.Status),
		Generation:  t.Generation,
		Version:     t.Version,
		LogicalTime: t.LogicalTime,
	}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// canonicalJSON marshals a value to a deterministic JSON document for
// deduplication hashing.
func canonicalJSON(v any) ([]byte, error) { return json.Marshal(v) }

// dedupResolve resolves operation-idempotency for a command. It returns the
// stored response on a same-content replay, or an error on a content conflict.
func dedupResolve(ctx context.Context, d *store.DedupRepo, taskID, opID string, generation int, content []byte) ([]byte, bool, error) {
	prior, ok, err := d.Lookup(ctx, taskID, opID, generation)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	hash := task.NormalizeContent(content)
	if prior.ContentHash != hash {
		return nil, false, ErrOperationConflict
	}
	return []byte(prior.ResponseJSON), true, nil
}

// advanceStatus checks that the task may move from its current state to next
// and returns the updated status; it is a pure state-machine guard.
func advanceStatus(t *task.IncubationTask, next task.TaskStatus) error {
	if t.Status.Terminal() {
		return ErrTerminal
	}
	if !t.Status.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidState, t.Status, next)
	}
	t.Status = next
	t.LogicalTime++
	return nil
}
