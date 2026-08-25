package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// ErrVersionConflict is returned when an optimistic save finds a stale version.
var ErrVersionConflict = errors.New("store: task version conflict")

// TaskRepo implements task.Repository over the SQL database.
type TaskRepo struct{ db DBTX }

// NewTaskRepo builds a task repository over the given query surface.
func NewTaskRepo(db DBTX) *TaskRepo { return &TaskRepo{db: db} }

// Load fetches a task by id, decoding its receiver list.
func (r *TaskRepo) Load(ctx context.Context, id string) (task.IncubationTask, error) {
	var t task.IncubationTask
	var receivers string
	var created int64
	var finalKind string
	err := r.db.QueryRowContext(ctx, `SELECT id, batch_no, status, generation, rule_snapshot,
		fumigation_digest, house_id, shift_id, receivers, version, logical_time, created_at,
		final_kind, final_version FROM incubation_task WHERE id=?`, id).
		Scan(&t.ID, &t.BatchNo, &t.Status, &t.Generation, &t.RuleSnapshot,
			&t.FumigationDigest, &t.HouseID, &t.ShiftID, &receivers, &t.Version,
			&t.LogicalTime, &created, &finalKind, &t.FinalVersion)
	if err != nil {
		return t, err
	}
	t.CreatedAt = time.Unix(created, 0)
	if err := json.Unmarshal([]byte(receivers), &t.Receivers); err != nil {
		return t, err
	}
	if finalKind != "" {
		t.FinalKind = finalKind
	}
	return t, nil
}

// Save persists a task using the Version field for optimistic concurrency. A
// stale version yields ErrVersionConflict and must be retried by the caller.
func (r *TaskRepo) Save(ctx context.Context, t task.IncubationTask) error {
	receivers, _ := json.Marshal(t.Receivers)
	created := t.CreatedAt.Unix()
	if created == 0 {
		created = nowUnix()
	}
	finalKind := ""
	if t.FinalKind != "" {
		finalKind = t.FinalKind
	}
	res, err := r.db.ExecContext(ctx, `UPDATE incubation_task SET batch_no=?, status=?, generation=?,
		rule_snapshot=?, fumigation_digest=?, house_id=?, shift_id=?, receivers=?, version=version+1,
		logical_time=?, created_at=?, final_kind=?, final_version=? WHERE id=? AND version=?`,
		t.BatchNo, string(t.Status), t.Generation, t.RuleSnapshot, t.FumigationDigest,
		t.HouseID, t.ShiftID, string(receivers), t.LogicalTime, created, finalKind,
		t.FinalVersion, t.ID, t.Version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrVersionConflict
	}
	t.Version++
	return nil
}

// Insert creates a brand-new task row. It is used only for the initial draft.
func (r *TaskRepo) Insert(ctx context.Context, t task.IncubationTask) error {
	receivers, _ := json.Marshal(t.Receivers)
	created := t.CreatedAt.Unix()
	if created == 0 {
		created = nowUnix()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO incubation_task
		(id, batch_no, status, generation, rule_snapshot, fumigation_digest, house_id, shift_id,
		 receivers, version, logical_time, created_at, final_kind, final_version)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.BatchNo, string(t.Status), t.Generation, t.RuleSnapshot, t.FumigationDigest,
		t.HouseID, t.ShiftID, string(receivers), t.Version, t.LogicalTime, created, "", 0)
	return err
}

// ReceiptRepo stores dual-person receipt confirmations.
type ReceiptRepo struct{ db DBTX }

// NewReceiptRepo builds a receipt repository.
func NewReceiptRepo(db DBTX) *ReceiptRepo { return &ReceiptRepo{db: db} }

// Add records one receiver's confirmation, ignoring duplicate rows.
func (r *ReceiptRepo) Add(ctx context.Context, taskID, personID string, generation int) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO receipt_confirmation
		(task_id, person_id, generation) VALUES (?,?,?)`, taskID, personID, generation)
	return err
}

// List returns the distinct confirmed receivers for a task generation.
func (r *ReceiptRepo) List(ctx context.Context, taskID string, generation int) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT person_id FROM receipt_confirmation
		WHERE task_id=? AND generation=? ORDER BY person_id`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DedupRepo stores operation-idempotency records.
type DedupRepo struct{ db DBTX }

// NewDedupRepo builds a deduplication repository.
func NewDedupRepo(db DBTX) *DedupRepo { return &DedupRepo{db: db} }

// Lookup returns a prior record for an operation id and generation.
func (r *DedupRepo) Lookup(ctx context.Context, taskID, opID string, generation int) (task.OperationDedup, bool, error) {
	var d task.OperationDedup
	err := r.db.QueryRowContext(ctx, `SELECT task_id, operation_id, generation, content_hash, response_json
		FROM operation_dedup WHERE task_id=? AND operation_id=? AND generation=?`,
		taskID, opID, generation).Scan(&d.TaskID, &d.OperationID, &d.Generation, &d.ContentHash, &d.ResponseJSON)
	if err == sql.ErrNoRows {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	return d, true, nil
}

// Insert records a new operation-idempotency entry.
func (r *DedupRepo) Insert(ctx context.Context, d task.OperationDedup) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO operation_dedup
		(task_id, operation_id, generation, content_hash, response_json) VALUES (?,?,?,?,?)`,
		d.TaskID, d.OperationID, d.Generation, d.ContentHash, d.ResponseJSON)
	return err
}

// AuditRepo appends audit-log entries.
type AuditRepo struct{ db DBTX }

// NewAuditRepo builds an audit repository.
func NewAuditRepo(db DBTX) *AuditRepo { return &AuditRepo{db: db} }

// Append records one audit event.
func (r *AuditRepo) Append(ctx context.Context, taskID, opID, event, detail string, logicalTime int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_log
		(task_id, operation_id, event, detail, logical_time, created_at) VALUES (?,?,?,?,?,?)`,
		taskID, opID, event, detail, logicalTime, nowUnix())
	return err
}

// AuditEntry is one audit-log row returned to the API.
type AuditEntry struct {
	TaskID      string `json:"task_id"`
	OperationID string `json:"operation_id"`
	Event       string `json:"event"`
	Detail      string `json:"detail"`
	LogicalTime int64  `json:"logical_time"`
	CreatedAt   int64  `json:"created_at"`
}

// ListAudit returns the audit trail for a task in insertion order.
func (r *AuditRepo) List(ctx context.Context, taskID string) ([]AuditEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, operation_id, event, detail, logical_time, created_at
		FROM audit_log WHERE task_id=? ORDER BY seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.TaskID, &e.OperationID, &e.Event, &e.Detail, &e.LogicalTime, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

var _ task.Repository = (*TaskRepo)(nil)
