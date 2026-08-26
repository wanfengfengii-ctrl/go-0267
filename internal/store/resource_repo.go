package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
)

// ResourceRepo implements resource.Repository plus the seal/blind persistence.
type ResourceRepo struct{ db DBTX }

// NewResourceRepo builds a resource repository.
func NewResourceRepo(db DBTX) *ResourceRepo { return &ResourceRepo{db: db} }

// Acquire inserts a lease; a conflicting open lease yields ErrLeaseConflict.
func (r *ResourceRepo) Acquire(ctx context.Context, lease resource.ResourceLease) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO resource_lease
		(lease_type, resource_key, task_id, generation, acquired_at, expires_at, release_reason)
		VALUES (?,?,?,?,?,?,?)`,
		string(lease.Type), lease.ResourceKey, lease.TaskID, lease.Generation,
		lease.AcquiredAt, lease.ExpiresAt, lease.ReleaseReason)
	if err != nil {
		if isUniqueViolation(err) {
			return resource.ErrLeaseConflict
		}
		return err
	}
	return nil
}

// Release deletes an open lease by type and key (idempotent).
func (r *ResourceRepo) Release(ctx context.Context, leaseType resource.LeaseType, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM resource_lease WHERE lease_type=? AND resource_key=?`,
		string(leaseType), key)
	return err
}

// Active returns all open leases for a task.
func (r *ResourceRepo) Active(ctx context.Context, taskID string) ([]resource.ResourceLease, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT lease_type, resource_key, task_id, generation,
		acquired_at, expires_at, release_reason FROM resource_lease WHERE task_id=? ORDER BY lease_type, resource_key`,
		taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resource.ResourceLease
	for rows.Next() {
		var l resource.ResourceLease
		var lt string
		if err := rows.Scan(&lt, &l.ResourceKey, &l.TaskID, &l.Generation,
			&l.AcquiredAt, &l.ExpiresAt, &l.ReleaseReason); err != nil {
			return nil, err
		}
		l.Type = resource.LeaseType(lt)
		out = append(out, l)
	}
	return out, rows.Err()
}

// HoldSeal persists a tray seal and its ordered positions.
func (r *ResourceRepo) HoldSeal(ctx context.Context, taskID, sealNo string, positions []resource.TrayPosition) error {
	data, _ := json.Marshal(positions)
	_, err := r.db.ExecContext(ctx, `INSERT INTO tray_seal (task_id, seal_no, positions) VALUES (?,?,?)`,
		taskID, sealNo, string(data))
	return err
}

// Seals returns the tray seals bound to a task.
func (r *ResourceRepo) Seals(ctx context.Context, taskID string) ([]resource.TraySeal, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT seal_no, positions FROM tray_seal WHERE task_id=? ORDER BY seal_no`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resource.TraySeal
	for rows.Next() {
		var s resource.TraySeal
		var pos string
		if err := rows.Scan(&s.SealNo, &pos); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pos), &s.Positions); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// HoldBlind persists a blind sample.
func (r *ResourceRepo) HoldBlind(ctx context.Context, taskID, code, digest string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO blind_sample (task_id, code, digest, revealed, reveal_version)
		VALUES (?,?,?,0,0)`, taskID, code, digest)
	return err
}

// Blinds returns the blind samples bound to a task without leaking digests.
func (r *ResourceRepo) Blinds(ctx context.Context, taskID string) ([]resource.BlindSample, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT code, digest, revealed, reveal_version FROM blind_sample
		WHERE task_id=? ORDER BY code`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resource.BlindSample
	for rows.Next() {
		var b resource.BlindSample
		var revealed int
		if err := rows.Scan(&b.Code, &b.Digest, &revealed, &b.RevealVersion); err != nil {
			return nil, err
		}
		b.Revealed = revealed == 1
		out = append(out, b)
	}
	return out, rows.Err()
}

// RevealBlind marks a blind sample as revealed with a version.
func (r *ResourceRepo) RevealBlind(ctx context.Context, taskID, code string, version int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE blind_sample SET revealed=1, reveal_version=?
		WHERE task_id=? AND code=? AND revealed=0`, version, taskID, code)
	if err != nil {
		return err
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "constraint failed"))
}

var _ resource.Repository = (*ResourceRepo)(nil)
