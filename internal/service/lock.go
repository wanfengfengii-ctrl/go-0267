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

// leaseTTL is the logical-time distance before a lease may expire. It is far
// beyond any single task's logical clock, so leases only end by explicit
// release (cancel/terminal) and never by wall-clock drift.
const leaseTTL int64 = 1_000_000

// CreateTaskRequest creates a draft task in the pending-lock state.
type CreateTaskRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
}

// CreateTask creates the draft aggregate and returns its id.
func (s *Service) CreateTask(ctx context.Context, req CreateTaskRequest) (CommandResult, error) {
	if req.Generation == 0 {
		req.Generation = 1
	}
	id := newID()
	draft := task.IncubationTask{
		ID:          id,
		Status:      task.StatusPendingLock,
		Generation:  req.Generation,
		Version:     1,
		LogicalTime: 1,
	}
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		if err := tr.Insert(ctx, draft); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, id, req.OperationID, "created", "", draft.LogicalTime); err != nil {
			return err
		}
		out = newResult(draft)
		return nil
	})
	return out, err
}

// SealSpec describes one tray seal and its ordered positions.
type SealSpec struct {
	SealNo    string `json:"seal_no"`
	Positions []int  `json:"positions"`
}

// LockTaskRequest atomically binds source, rules, seals, positions, samples
// and every initial resource lease to the task.
type LockTaskRequest struct {
	OperationID       string     `json:"operation_id"`
	Generation        int        `json:"generation"`
	HouseID           string     `json:"house_id"`
	ShiftID           string     `json:"shift_id"`
	FumigationBatchID string     `json:"fumigation_batch_id"`
	FumigationDigest  string     `json:"fumigation_digest"`
	RuleSetVersion    int        `json:"rule_set_version"`
	BatchNo           string     `json:"batch_no"`
	IncubatorSlotID   string     `json:"incubator_slot_id"`
	CandlingWindowID  string     `json:"candling_window_id"`
	Seals             []SealSpec `json:"seals"`
	BlindCodes        []string   `json:"blind_codes"`
	CultureWells      []string   `json:"culture_wells"`
	RapidWells        []string   `json:"rapid_wells"`
}

// LockTask runs the single-transaction lock described by acceptance item 1.
func (s *Service) LockTask(ctx context.Context, taskID string, req LockTaskRequest) (CommandResult, error) {
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
		if t.Status != task.StatusPendingLock {
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
		if _, _, err := catalog.ValidateSource(ctx, cat, req.HouseID, req.ShiftID); err != nil {
			return err
		}
		if _, err := catalog.ValidateFumigation(ctx, cat, req.FumigationBatchID, req.FumigationDigest); err != nil {
			return err
		}
		if _, err := cat.RuleSet(ctx, req.RuleSetVersion); err != nil {
			return catalog.ErrRuleSetNotFound
		}

		rr := store.NewResourceRepo(tx)
		base := t.LogicalTime + 1
		leases := []resource.ResourceLease{
			{Type: resource.LeaseBatchNo, ResourceKey: req.BatchNo},
			{Type: resource.LeaseIncubatorSlot, ResourceKey: req.IncubatorSlotID},
			{Type: resource.LeaseCandlingWindow, ResourceKey: req.CandlingWindowID},
		}
		for _, seal := range req.Seals {
			leases = append(leases, resource.ResourceLease{Type: resource.LeaseTraySeal, ResourceKey: seal.SealNo})
		}
		for _, b := range req.BlindCodes {
			leases = append(leases, resource.ResourceLease{Type: resource.LeaseBlindCode, ResourceKey: b})
		}
		for _, w := range req.CultureWells {
			leases = append(leases, resource.ResourceLease{Type: resource.LeaseCultureWell, ResourceKey: w})
		}
		for _, w := range req.RapidWells {
			leases = append(leases, resource.ResourceLease{Type: resource.LeaseRapidTestWell, ResourceKey: w})
		}
		for i := range leases {
			leases[i].TaskID = taskID
			leases[i].Generation = req.Generation
			leases[i].AcquiredAt = base
			leases[i].ExpiresAt = base + leaseTTL
			if err := rr.Acquire(ctx, leases[i]); err != nil {
				return err
			}
		}

		for _, seal := range req.Seals {
			positions := make([]resource.TrayPosition, 0, len(seal.Positions))
			for _, p := range seal.Positions {
				positions = append(positions, resource.TrayPosition{Position: p})
			}
			if err := rr.HoldSeal(ctx, taskID, seal.SealNo, positions); err != nil {
				return err
			}
		}
		for _, b := range req.BlindCodes {
			if err := rr.HoldBlind(ctx, taskID, b, blindDigest(b)); err != nil {
				return err
			}
		}

		t.BatchNo = req.BatchNo
		t.HouseID = req.HouseID
		t.ShiftID = req.ShiftID
		t.FumigationDigest = req.FumigationDigest
		t.RuleSnapshot = req.RuleSetVersion
		t.LogicalTime = base
		if err := advanceStatus(&t, task.StatusPendingReceipt); err != nil {
			return err
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}

		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "locked", "source and leases bound", t.LogicalTime); err != nil {
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

func blindDigest(s string) string {
	return "blind-" + s + "-digest"
}
