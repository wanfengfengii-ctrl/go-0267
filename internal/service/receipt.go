package service

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// AddReceiptRequest records one receiver's physical confirmation.
type AddReceiptRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	PersonID    string `json:"person_id"`
}

// AddReceipt confirms one qualified receiver for the current generation.
func (s *Service) AddReceipt(ctx context.Context, taskID string, req AddReceiptRequest) (CommandResult, error) {
	content, _ := canonicalJSON(req)
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != req.Generation {
			return ErrStaleGeneration
		}
		if t.Status != task.StatusPendingReceipt {
			return ErrInvalidState
		}
		drepo := store.NewDedupRepo(tx)
		if replay, ok, err := dedupResolve(ctx, drepo, taskID, req.OperationID, req.Generation, content); err != nil {
			return err
		} else if ok {
			_ = json.Unmarshal(replay, &out)
			return nil
		}

		cat := store.NewCatalogRepo(tx)
		if err := catalog.ValidateRole(ctx, cat, req.PersonID, catalog.RoleReceiver); err != nil {
			return ErrNotQualified
		}

		rr := store.NewReceiptRepo(tx)
		if err := rr.Add(ctx, taskID, req.PersonID, req.Generation); err != nil {
			return err
		}
		t.LogicalTime++
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "receipt", req.PersonID, t.LogicalTime); err != nil {
			return err
		}
		out = newResult(t)
		resp, _ := json.Marshal(out)
		return drepo.Insert(ctx, task.OperationDedup{
			TaskID: taskID, OperationID: req.OperationID, Generation: req.Generation,
			ContentHash: task.NormalizeContent(content), ResponseJSON: string(resp),
		})
	})
	return out, err
}

// StartRequest atomically starts collection once two distinct receivers have
// confirmed the physical eggs and seals.
type StartRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
}

// Start transitions pending_receipt -> resources_occupied after dual receipt.
func (s *Service) Start(ctx context.Context, taskID string, req StartRequest) (CommandResult, error) {
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != req.Generation {
			return ErrStaleGeneration
		}
		if t.Status != task.StatusPendingReceipt {
			return ErrInvalidState
		}
		rr := store.NewReceiptRepo(tx)
		receipts, err := rr.List(ctx, taskID, req.Generation)
		if err != nil {
			return err
		}
		if len(receipts) < 2 {
			return ErrInvalidState
		}
		t.Receivers = receipts
		if err := advanceStatus(&t, task.StatusResourcesOccupied); err != nil {
			return err
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "started", "dual receipt complete", t.LogicalTime); err != nil {
			return err
		}
		out = newResult(t)
		return nil
	})
	return out, err
}

// ExchangeRequest atomically swaps a reservable window/slot lease.
type ExchangeRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	Type        string `json:"type"`
	FromKey     string `json:"from_key"`
	ToKey       string `json:"to_key"`
}

// ExchangeWindow validates the target then swaps the lease in one transaction.
func (s *Service) ExchangeWindow(ctx context.Context, taskID string, req ExchangeRequest) (CommandResult, error) {
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != req.Generation {
			return ErrStaleGeneration
		}

		x := resource.Exchange{
			Type:       resource.LeaseType(req.Type),
			FromKey:    req.FromKey,
			ToKey:      req.ToKey,
			TaskID:     taskID,
			Generation: req.Generation,
		}
		if err := x.Validate(); err != nil {
			return err
		}

		cat := store.NewCatalogRepo(tx)
		switch x.Type {
		case resource.LeaseIncubatorSlot:
			if _, err := cat.Slot(ctx, x.ToKey); err != nil {
				return err
			}
		case resource.LeaseCandlingWindow:
			if _, err := cat.Window(ctx, x.ToKey); err != nil {
				return err
			}
		}

		rr := store.NewResourceRepo(tx)
		if err := rr.Release(ctx, x.Type, x.FromKey); err != nil {
			return err
		}
		t.LogicalTime++
		if err := rr.Acquire(ctx, resource.ResourceLease{
			Type: x.Type, ResourceKey: x.ToKey, TaskID: taskID, Generation: req.Generation,
			AcquiredAt: t.LogicalTime, ExpiresAt: t.LogicalTime + leaseTTL,
		}); err != nil {
			return err
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "window_exchange",
			string(x.Type)+":"+x.FromKey+"->"+x.ToKey, t.LogicalTime); err != nil {
			return err
		}
		out = newResult(t)
		return nil
	})
	return out, err
}
