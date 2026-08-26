package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// ErrBlindPremature is returned when blind codes are revealed before the
// review stage.
var ErrBlindPremature = errors.New("service: blind reveal not yet allowed")

// RevealBlindRequest marks blind samples as revealed at the review stage.
type RevealBlindRequest struct {
	OperationID string   `json:"operation_id"`
	Generation  int      `json:"generation"`
	Codes       []string `json:"codes"`
}

// RevealBlind reveals the given blind codes; reveal is only allowed once
// collection is complete and never leaks the codes before that point.
func (s *Service) RevealBlind(ctx context.Context, taskID string, req RevealBlindRequest) (CommandResult, error) {
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
		if t.Status != task.StatusPendingReview && t.Status != task.StatusAdmittable {
			return ErrBlindPremature
		}
		rr := store.NewResourceRepo(tx)
		for _, code := range req.Codes {
			if err := rr.RevealBlind(ctx, taskID, code, t.Generation); err != nil {
				return err
			}
		}
		t.LogicalTime++
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, req.OperationID, "blind_revealed", "codes revealed", t.LogicalTime)
		out = newResult(t)
		return nil
	})
	return out, err
}

// CreateRetestRequest opens the single active retest case for the generation.
type CreateRetestRequest struct {
	OperationID       string   `json:"operation_id"`
	Generation        int      `json:"generation"`
	Trigger           string   `json:"trigger"`
	AffectedSeals     []string `json:"affected_seals"`
	AffectedPositions []int    `json:"affected_positions"`
	AffectedBlinds    []string `json:"affected_blinds"`
	AffectedWells     []string `json:"affected_wells"`
}

// CreateRetest creates the sole active retest case for the current generation.
// A second active case for the same generation is rejected by the unique key.
func (s *Service) CreateRetest(ctx context.Context, taskID string, req CreateRetestRequest) (CommandResult, error) {
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
		ar := store.NewArbitrationRepo(tx)
		c := arbitration.RetestCase{
			TaskID: taskID, Generation: req.Generation,
			AffectedSeals: req.AffectedSeals, AffectedPositions: req.AffectedPositions,
			AffectedBlinds: req.AffectedBlinds, AffectedWells: req.AffectedWells,
			Verdict: string(arbitration.VerdictPending), Active: true,
		}
		if err := ar.PutRetest(ctx, c); err != nil {
			if isConflict(err) {
				return ErrInvalidState
			}
			return err
		}
		t.LogicalTime++
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, req.OperationID, "retest_opened", req.Trigger, t.LogicalTime)
		out = newResult(t)
		return nil
	})
	return out, err
}

// RetestEvidenceRequest appends a versioned evidence point to the active retest
// case, optionally resolving it with a verdict.
type RetestEvidenceRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	Kind        string `json:"kind"`
	Value       int64  `json:"value"`
	Verdict     string `json:"verdict,omitempty"`
}

// AddRetestEvidence appends retest evidence and resolves the case when a
// verdict is supplied.
func (s *Service) AddRetestEvidence(ctx context.Context, taskID string, req RetestEvidenceRequest) (CommandResult, error) {
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
		ar := store.NewArbitrationRepo(tx)
		if _, ok, _ := ar.Retest(ctx, taskID, req.Generation); !ok {
			return ErrInvalidState
		}
		er := store.NewEvidenceRepo(tx)
		prior, _ := er.RetestEvidence(ctx, taskID, req.Generation)
		version := 1
		for _, vs := range prior {
			if len(vs) >= version {
				version = len(vs) + 1
			}
		}
		if version <= 1 {
			version = 1
		}
		if err := er.PutRetestEvidence(ctx, taskID, req.Generation, req.Kind, req.Value, version); err != nil {
			return err
		}
		if req.Verdict != "" {
			if err := ar.ResolveRetest(ctx, taskID, req.Generation, req.Verdict); err != nil {
				return err
			}
		}
		t.LogicalTime++
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, req.OperationID, "retest_evidence", req.Kind, t.LogicalTime)
		out = newResult(t)
		return nil
	})
	return out, err
}

func isConflict(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "constraint failed"))
}
