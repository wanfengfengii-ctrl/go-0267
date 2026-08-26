package service

import (
	"context"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// TaskView is the aggregated task query response.
type TaskView struct {
	ID               string   `json:"id"`
	BatchNo          string   `json:"batch_no"`
	Status           string   `json:"status"`
	Generation       int      `json:"generation"`
	RuleSnapshot     int      `json:"rule_snapshot"`
	FumigationDigest string   `json:"fumigation_digest"`
	HouseID          string   `json:"house_id"`
	ShiftID          string   `json:"shift_id"`
	Receivers        []string `json:"receivers"`
	Version          int64    `json:"version"`
	LogicalTime      int64    `json:"logical_time"`
	FinalKind        string   `json:"final_kind,omitempty"`
}

// GetTask returns the aggregated task view.
func (s *Service) GetTask(ctx context.Context, id string) (TaskView, error) {
	tr := store.NewTaskRepo(s.store.DB())
	t, err := tr.Load(ctx, id)
	if err != nil {
		return TaskView{}, ErrTaskNotFound
	}
	return TaskView{
		ID: t.ID, BatchNo: t.BatchNo, Status: string(t.Status), Generation: t.Generation,
		RuleSnapshot: t.RuleSnapshot, FumigationDigest: t.FumigationDigest,
		HouseID: t.HouseID, ShiftID: t.ShiftID, Receivers: t.Receivers,
		Version: t.Version, LogicalTime: t.LogicalTime, FinalKind: t.FinalKind,
	}, nil
}

// EvidenceView is the aggregated evidence query response.
type EvidenceView struct {
	Candling        []evidence.CandlingEntry           `json:"candling"`
	Culture         []arbitration.CultureEvidence      `json:"culture"`
	RapidTest       []arbitration.RapidTestEvidence    `json:"rapid_test"`
	Physicochemical []evidence.PhysicochemicalEvidence `json:"physicochemical"`
	Retest          *RetestView                        `json:"retest,omitempty"`
	DeviceAttempts  []evidence.DeviceAttempt           `json:"device_attempts"`
	BlindsRevealed  bool                               `json:"blinds_revealed"`
}

// RetestView exposes the active retest case without leaking blind codes.
type RetestView struct {
	Generation        int      `json:"generation"`
	AffectedSeals     []string `json:"affected_seals"`
	AffectedPositions []int    `json:"affected_positions"`
	AffectedWells     []string `json:"affected_wells"`
	Verdict           string   `json:"verdict"`
	Active            bool     `json:"active"`
}

// GetEvidence aggregates the current evidence for a task.
func (s *Service) GetEvidence(ctx context.Context, id string) (EvidenceView, error) {
	var out EvidenceView
	db := s.store.DB()
	er := store.NewEvidenceRepo(db)
	ar := store.NewArbitrationRepo(db)
	rr := store.NewResourceRepo(db)

	candling, err := er.Candling(ctx, id)
	if err != nil {
		return out, err
	}
	out.Candling = candling
	culture, err := ar.Culture(ctx, id)
	if err != nil {
		return out, err
	}
	out.Culture = culture
	rapid, err := ar.RapidTest(ctx, id)
	if err != nil {
		return out, err
	}
	out.RapidTest = rapid
	physico, err := er.Physicochemical(ctx, id)
	if err != nil {
		return out, err
	}
	out.Physicochemical = physico
	attempts, err := er.Attempts(ctx, id)
	if err != nil {
		return out, err
	}
	out.DeviceAttempts = attempts

	tr := store.NewTaskRepo(db)
	t, err := tr.Load(ctx, id)
	if err != nil {
		return out, err
	}
	rc, active, err := ar.Retest(ctx, id, t.Generation)
	if err != nil {
		return out, err
	}
	if active {
		out.Retest = &RetestView{
			Generation: rc.Generation, AffectedSeals: rc.AffectedSeals,
			AffectedPositions: rc.AffectedPositions, AffectedWells: rc.AffectedWells,
			Verdict: rc.Verdict, Active: rc.Active,
		}
	}
	blinds, err := rr.Blinds(ctx, id)
	if err != nil {
		return out, err
	}
	for _, b := range blinds {
		if b.Revealed {
			out.BlindsRevealed = true
			break
		}
	}
	return out, nil
}

// LeaseView is one lease row in the lease query response.
type LeaseView struct {
	Type        string `json:"type"`
	ResourceKey string `json:"resource_key"`
	Generation  int    `json:"generation"`
	AcquiredAt  int64  `json:"acquired_at"`
	ExpiresAt   int64  `json:"expires_at"`
}

// GetLeases returns the active resource leases for a task.
func (s *Service) GetLeases(ctx context.Context, id string) ([]LeaseView, error) {
	rr := store.NewResourceRepo(s.store.DB())
	leases, err := rr.Active(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]LeaseView, 0, len(leases))
	for _, l := range leases {
		out = append(out, LeaseView{
			Type: string(l.Type), ResourceKey: l.ResourceKey, Generation: l.Generation,
			AcquiredAt: l.AcquiredAt, ExpiresAt: l.ExpiresAt,
		})
	}
	return out, nil
}

// GetAudit returns the audit trail for a task.
func (s *Service) GetAudit(ctx context.Context, id string) ([]store.AuditEntry, error) {
	ar := store.NewAuditRepo(s.store.DB())
	return ar.List(ctx, id)
}

// GetCredential returns the immutable credential for an admitted task.
func (s *Service) GetCredential(ctx context.Context, id string) (arbitration.IncubationCredential, error) {
	sw := store.NewSingleWriterRepo(s.store.DB())
	cred, err := sw.Credential(ctx, id)
	if err != nil {
		return arbitration.IncubationCredential{}, ErrTaskNotFound
	}
	return cred, nil
}

// PendingRetries returns unresolved device attempts for a task. This is the
// restart-recovery surface: after reopening the database the same attempts are
// still present and may be retried deterministically.
func (s *Service) PendingRetries(ctx context.Context, id string) ([]evidence.DeviceAttempt, error) {
	er := store.NewEvidenceRepo(s.store.DB())
	return er.PendingAttempts(ctx, id)
}

// OpenTasks lists tasks that are not yet terminal (restart recovery).
func (s *Service) OpenTasks(ctx context.Context) ([]task.IncubationTask, error) {
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id FROM incubation_task
		WHERE status NOT IN ('admitted','isolated','cancelled') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []task.IncubationTask
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		t, err := store.NewTaskRepo(s.store.DB()).Load(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
