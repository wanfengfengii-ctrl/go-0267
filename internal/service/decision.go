package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// AddReviewRequest records one independent reviewer's signed conclusion.
type AddReviewRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	PersonID    string `json:"person_id"`
	Decision    string `json:"decision"`
}

// AddReview signs a qualified, non-receiver reviewer and advances the task to
// admittable once two distinct passing reviews exist.
func (s *Service) AddReview(ctx context.Context, taskID string, req AddReviewRequest) (CommandResult, error) {
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
		if t.Status != task.StatusPendingReview {
			return ErrInvalidState
		}

		cat := store.NewCatalogRepo(tx)
		if err := catalog.ValidateRole(ctx, cat, req.PersonID, catalog.RoleReviewer); err != nil {
			return ErrNotQualified
		}

		ar := store.NewArbitrationRepo(tx)
		existing, _ := ar.Reviews(ctx, taskID, req.Generation)
		policy := arbitration.NewReviewPolicy(t.Receivers, reviewPersons(existing))
		if err := policy.Validate(req.PersonID); err != nil {
			return err
		}

		decision := arbitration.ReviewDecision(req.Decision)
		if decision != arbitration.ReviewPass && decision != arbitration.ReviewFail {
			return errors.New("service: invalid review decision")
		}
		if err := ar.PutReview(ctx, arbitration.IndependentReview{
			TaskID: taskID, PersonID: req.PersonID, Generation: req.Generation, Decision: decision,
		}); err != nil {
			return err
		}

		t.LogicalTime++
		if decision == arbitration.ReviewPass && passingReviews(existing)+1 >= 2 {
			if err := advanceStatus(&t, task.StatusAdmittable); err != nil {
				return err
			}
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, req.OperationID, "review", string(decision), t.LogicalTime)
		out = newResult(t)
		return nil
	})
	return out, err
}

func reviewPersons(rs []arbitration.IndependentReview) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.PersonID)
	}
	return out
}

func passingReviews(rs []arbitration.IndependentReview) int {
	n := 0
	for _, r := range rs {
		if r.Decision == arbitration.ReviewPass {
			n++
		}
	}
	return n
}

// FinalDecisionRequest submits one of admit, isolate or cancel into the
// single-write terminal race.
type FinalDecisionRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	Kind        string `json:"kind"`
	PersonID    string `json:"person_id,omitempty"`
}

// FinalDecision commits the terminal outcome. Only the first competing command
// wins; late or ordinary operations on a terminal task are rejected.
func (s *Service) FinalDecision(ctx context.Context, taskID string, req FinalDecisionRequest) (CommandResult, error) {
	kind := arbitration.FinalDecisionKind(req.Kind)
	switch kind {
	case arbitration.DecisionAdmit, arbitration.DecisionIsolate, arbitration.DecisionCancel:
	default:
		return CommandResult{}, errors.New("service: invalid decision kind")
	}

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

		switch kind {
		case arbitration.DecisionAdmit, arbitration.DecisionIsolate:
			if t.Status != task.StatusAdmittable {
				return ErrInvalidState
			}
			if err := requireClosed(ctx, tx, t); err != nil {
				return err
			}
		case arbitration.DecisionCancel:
			cat := store.NewCatalogRepo(tx)
			if err := catalog.ValidateRole(ctx, cat, req.PersonID, catalog.RoleAuthorizer); err != nil {
				return ErrNotQualified
			}
		}

		sw := store.NewSingleWriterRepo(tx)
		_, won, err := sw.CommitFinal(ctx, arbitration.FinalDecision{
			TaskID: taskID, Kind: kind, Version: t.Version, EvidenceDigest: t.BatchNo,
		})
		if err != nil {
			return err
		}
		if !won {
			return ErrTerminal
		}

		switch kind {
		case arbitration.DecisionAdmit:
			if err := advanceStatus(&t, task.StatusAdmitted); err != nil {
				return err
			}
		case arbitration.DecisionIsolate:
			if err := advanceStatus(&t, task.StatusIsolated); err != nil {
				return err
			}
		case arbitration.DecisionCancel:
			if err := advanceStatus(&t, task.StatusCancelled); err != nil {
				return err
			}
		}
		t.FinalKind = string(kind)
		t.FinalVersion = t.Version
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, req.OperationID, "final_"+string(kind), "terminal decision", t.LogicalTime)
		out = newResult(t)
		return nil
	})
	return out, err
}

// requireClosed evaluates the evidence closure and returns
// arbitration.ErrEvidenceIncomplete when any mandatory collection or retest
// stage is incomplete.
func requireClosed(ctx context.Context, tx *sql.Tx, t task.IncubationTask) error {
	c, err := evaluateClosure(ctx, tx, t)
	if err != nil {
		return err
	}
	if !c.Complete() {
		return arbitration.ErrEvidenceIncomplete
	}
	return nil
}

// evaluateClosure computes the current evidence closure from the ledger.
func evaluateClosure(ctx context.Context, tx *sql.Tx, t task.IncubationTask) (arbitration.EvidenceClosure, error) {
	var c arbitration.EvidenceClosure
	er := store.NewEvidenceRepo(tx)
	rr := store.NewResourceRepo(tx)
	ar := store.NewArbitrationRepo(tx)

	seals, err := rr.Seals(ctx, t.ID)
	if err != nil {
		return c, err
	}
	total := 0
	for _, s := range seals {
		total += len(s.Positions)
	}
	candling, err := er.Candling(ctx, t.ID)
	if err != nil {
		return c, err
	}
	c.CandlingComplete = total > 0 && len(candling) >= total

	swabs, err := er.SwabSeals(ctx, t.ID)
	if err != nil {
		return c, err
	}
	c.SwabSealed = len(swabs) > 0

	leases, err := rr.Active(ctx, t.ID)
	if err != nil {
		return c, err
	}
	var cultureWells, rapidWells []string
	for _, l := range leases {
		switch l.Type {
		case resource.LeaseCultureWell:
			cultureWells = append(cultureWells, l.ResourceKey)
		case resource.LeaseRapidTestWell:
			rapidWells = append(rapidWells, l.ResourceKey)
		}
	}
	cultures, err := ar.Culture(ctx, t.ID)
	if err != nil {
		return c, err
	}
	c.CultureComplete = covers(cultureWells, readWells(cultures))
	rapids, err := ar.RapidTest(ctx, t.ID)
	if err != nil {
		return c, err
	}
	c.RapidTestComplete = covers(rapidWells, rapidWellsOf(rapids))

	physico, err := er.Physicochemical(ctx, t.ID)
	if err != nil {
		return c, err
	}
	cat := store.NewCatalogRepo(tx)
	rs, err := cat.RuleSet(ctx, t.RuleSnapshot)
	if err != nil {
		return c, err
	}
	c.PhysicochemicalOK = physicochemicalOK(physico, rs)

	_, active, err := ar.Retest(ctx, t.ID, t.Generation)
	if err != nil {
		return c, err
	}
	c.RetestsResolved = !active

	return c, nil
}

func rapidWellsOf(rs []arbitration.RapidTestEvidence) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Well)
	}
	return out
}

// physicochemicalOK requires each of the four physicochemical kinds to be
// present and within its locked threshold bound (domain rule 5).
func physicochemicalOK(ev []evidence.PhysicochemicalEvidence, rs catalog.RuleSetVersion) bool {
	values := map[catalog.EvidenceKind]int64{}
	for _, e := range ev {
		values[e.Kind] = e.Raw
	}
	for _, k := range []catalog.EvidenceKind{
		catalog.EvidenceEggWeight, catalog.EvidenceAirCell,
		catalog.EvidenceCleanliness, catalog.EvidenceFumigation,
	} {
		value, ok := values[k]
		if !ok {
			return false
		}
		th, ok := rs.Thresholds[k]
		if !ok {
			return false
		}
		if !arbitration.Within(value, th) {
			return false
		}
	}
	return true
}
