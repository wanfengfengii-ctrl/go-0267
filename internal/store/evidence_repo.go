package store

import (
	"context"
	"encoding/json"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
)

// EvidenceRepo persists the candling, physicochemical and device-attempt books.
type EvidenceRepo struct{ db DBTX }

// NewEvidenceRepo builds an evidence repository.
func NewEvidenceRepo(db DBTX) *EvidenceRepo { return &EvidenceRepo{db: db} }

// PutCandling stores one candling entry at version 1.
func (r *EvidenceRepo) PutCandling(ctx context.Context, e evidence.CandlingEntry) error {
	defects, _ := json.Marshal(e.Defects)
	_, err := r.db.ExecContext(ctx, `INSERT INTO candling_entry
		(task_id, seal_no, position, category, defects, retest, version) VALUES (?,?,?,?,?,?,?)`,
		e.TaskID, e.SealNo, e.Position, string(e.Category), string(defects), boolInt(e.Retest), e.Version)
	return err
}

// Candling returns all current candling entries for a task.
func (r *EvidenceRepo) Candling(ctx context.Context, taskID string) ([]evidence.CandlingEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, seal_no, position, category, defects, retest, version
		FROM candling_entry WHERE task_id=? ORDER BY seal_no, position`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.CandlingEntry
	for rows.Next() {
		var e evidence.CandlingEntry
		var defects string
		var retest int
		if err := rows.Scan(&e.TaskID, &e.SealNo, &e.Position, &e.Category, &defects, &retest, &e.Version); err != nil {
			return nil, err
		}
		e.Retest = retest == 1
		if err := json.Unmarshal([]byte(defects), &e.Defects); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PutPhysicochemical stores one fixed-point physicochemical measurement.
func (r *EvidenceRepo) PutPhysicochemical(ctx context.Context, e evidence.PhysicochemicalEvidence) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO physicochemical_evidence
		(task_id, seal_no, position, kind, raw, derived, version) VALUES (?,?,?,?,?,?,?)`,
		e.TaskID, e.SealNo, e.Position, string(e.Kind), e.Raw, boolInt(e.Derived), e.Version)
	return err
}

// Physicochemical returns all physicochemical evidence for a task.
func (r *EvidenceRepo) Physicochemical(ctx context.Context, taskID string) ([]evidence.PhysicochemicalEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, seal_no, position, kind, raw, derived, version
		FROM physicochemical_evidence WHERE task_id=? ORDER BY seal_no, position, kind`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.PhysicochemicalEvidence
	for rows.Next() {
		var e evidence.PhysicochemicalEvidence
		var derived int
		if err := rows.Scan(&e.TaskID, &e.SealNo, &e.Position, &e.Kind, &e.Raw, &derived, &e.Version); err != nil {
			return nil, err
		}
		e.Derived = derived == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// PutAttempt persists one device attempt (append-only).
func (r *EvidenceRepo) PutAttempt(ctx context.Context, a evidence.DeviceAttempt) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO device_attempt
		(task_id, device_id, kind, object, generation, attempt, failure, next_at, pending)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		a.TaskID, a.DeviceID, a.Kind, a.Object, a.Generation, a.Attempt,
		string(a.Failure), a.NextAt, boolInt(a.Pending))
	return err
}

// Attempts returns all device attempts for a task.
func (r *EvidenceRepo) Attempts(ctx context.Context, taskID string) ([]evidence.DeviceAttempt, error) {
	return r.queryAttempts(ctx, `SELECT task_id, device_id, kind, object, generation, attempt,
		failure, next_at, pending FROM device_attempt WHERE task_id=? ORDER BY device_id, kind, object, attempt`, taskID)
}

// MarkAttemptDone clears the pending flag on a resolved attempt.
func (r *EvidenceRepo) MarkAttemptDone(ctx context.Context, a evidence.DeviceAttempt) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_attempt SET pending=0 WHERE task_id=? AND device_id=?
		AND kind=? AND object=? AND generation=? AND attempt=?`,
		a.TaskID, a.DeviceID, a.Kind, a.Object, a.Generation, a.Attempt)
	return err
}

// PendingAttempts returns unresolved attempts for a task (restart recovery).
func (r *EvidenceRepo) PendingAttempts(ctx context.Context, taskID string) ([]evidence.DeviceAttempt, error) {
	return r.queryAttempts(ctx, `SELECT task_id, device_id, kind, object, generation, attempt,
		failure, next_at, pending FROM device_attempt WHERE task_id=? AND pending=1
		ORDER BY next_at, attempt`, taskID)
}

// queryAttempts runs a device-attempt query and scans the shared result shape.
func (r *EvidenceRepo) queryAttempts(ctx context.Context, query string, args ...any) ([]evidence.DeviceAttempt, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.DeviceAttempt
	for rows.Next() {
		var a evidence.DeviceAttempt
		var pending int
		if err := rows.Scan(&a.TaskID, &a.DeviceID, &a.Kind, &a.Object, &a.Generation,
			&a.Attempt, &a.Failure, &a.NextAt, &pending); err != nil {
			return nil, err
		}
		a.Pending = pending == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SealSwab records the swab-seal version for a seal.
func (r *EvidenceRepo) SealSwab(ctx context.Context, taskID, sealNo string, version int) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO swab_seal (task_id, seal_no, version) VALUES (?,?,?)`,
		taskID, sealNo, version)
	return err
}

// SwabSeals returns the sealed seals for a task.
func (r *EvidenceRepo) SwabSeals(ctx context.Context, taskID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT seal_no FROM swab_seal WHERE task_id=? ORDER BY seal_no`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PutRetestEvidence appends a retest evidence version.
func (r *EvidenceRepo) PutRetestEvidence(ctx context.Context, taskID string, generation int, kind string, value int64, version int) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO retest_evidence (task_id, generation, kind, value, version)
		VALUES (?,?,?,?,?)`, taskID, generation, kind, value, version)
	return err
}

// RetestEvidence returns the retest evidence chain for a task generation.
func (r *EvidenceRepo) RetestEvidence(ctx context.Context, taskID string, generation int) (map[string][]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT kind, value FROM retest_evidence WHERE task_id=? AND generation=?
		ORDER BY version`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var kind string
		var v int64
		if err := rows.Scan(&kind, &v); err != nil {
			return nil, err
		}
		out[kind] = append(out[kind], v)
	}
	return out, rows.Err()
}
