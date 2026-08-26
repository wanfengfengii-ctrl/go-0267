package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
)

// ArbitrationRepo persists culture/rapid version chains, retest cases, reviews,
// final decisions and the single credential.
type ArbitrationRepo struct{ db DBTX }

// NewArbitrationRepo builds an arbitration repository.
func NewArbitrationRepo(db DBTX) *ArbitrationRepo { return &ArbitrationRepo{db: db} }

// PutCulture appends a culture reading, demoting the previous current row.
func (r *ArbitrationRepo) PutCulture(ctx context.Context, e arbitration.CultureEvidence) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE culture_evidence SET current=0 WHERE task_id=? AND well=?`,
		e.TaskID, e.Well); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO culture_evidence
		(task_id, well, colony, version, current, raw_digest) VALUES (?,?,?,?,1,?)`,
		e.TaskID, e.Well, e.Colony, e.Version, e.RawDigest)
	return err
}

// Culture returns the current culture readings for a task.
func (r *ArbitrationRepo) Culture(ctx context.Context, taskID string) ([]arbitration.CultureEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, well, colony, version, current, raw_digest
		FROM culture_evidence WHERE task_id=? AND current=1 ORDER BY well`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []arbitration.CultureEvidence
	for rows.Next() {
		var e arbitration.CultureEvidence
		var cur int
		if err := rows.Scan(&e.TaskID, &e.Well, &e.Colony, &e.Version, &cur, &e.RawDigest); err != nil {
			return nil, err
		}
		e.Current = cur == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// PutRapidTest appends a rapid-test reading.
func (r *ArbitrationRepo) PutRapidTest(ctx context.Context, e arbitration.RapidTestEvidence) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE rapid_test_evidence SET current=0 WHERE task_id=? AND well=?`,
		e.TaskID, e.Well); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO rapid_test_evidence
		(task_id, well, ct_value, version, current, raw_digest) VALUES (?,?,?,?,1,?)`,
		e.TaskID, e.Well, e.CtValue, e.Version, e.RawDigest)
	return err
}

// RapidTest returns the current rapid-test readings for a task.
func (r *ArbitrationRepo) RapidTest(ctx context.Context, taskID string) ([]arbitration.RapidTestEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, well, ct_value, version, current, raw_digest
		FROM rapid_test_evidence WHERE task_id=? AND current=1 ORDER BY well`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []arbitration.RapidTestEvidence
	for rows.Next() {
		var e arbitration.RapidTestEvidence
		var cur int
		if err := rows.Scan(&e.TaskID, &e.Well, &e.CtValue, &e.Version, &cur, &e.RawDigest); err != nil {
			return nil, err
		}
		e.Current = cur == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// PutRetest inserts the single active retest case for a task generation.
func (r *ArbitrationRepo) PutRetest(ctx context.Context, c arbitration.RetestCase) error {
	seals, _ := json.Marshal(c.AffectedSeals)
	pos, _ := json.Marshal(c.AffectedPositions)
	blinds, _ := json.Marshal(c.AffectedBlinds)
	wells, _ := json.Marshal(c.AffectedWells)
	active := 0
	if c.Active {
		active = 1
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO retest_case
		(task_id, generation, affected_seals, affected_positions, affected_blinds, affected_wells, verdict, active)
		VALUES (?,?,?,?,?,?,?,?)`,
		c.TaskID, c.Generation, string(seals), string(pos), string(blinds), string(wells), c.Verdict, active)
	return err
}

// Retest returns the active retest case for a task generation, if any.
func (r *ArbitrationRepo) Retest(ctx context.Context, taskID string, generation int) (arbitration.RetestCase, bool, error) {
	var c arbitration.RetestCase
	var seals, pos, blinds, wells string
	var active int
	err := r.db.QueryRowContext(ctx, `SELECT task_id, generation, affected_seals, affected_positions,
		affected_blinds, affected_wells, verdict, active FROM retest_case
		WHERE task_id=? AND generation=? AND active=1`, taskID, generation).
		Scan(&c.TaskID, &c.Generation, &seals, &pos, &blinds, &wells, &c.Verdict, &active)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	_ = json.Unmarshal([]byte(seals), &c.AffectedSeals)
	_ = json.Unmarshal([]byte(pos), &c.AffectedPositions)
	_ = json.Unmarshal([]byte(blinds), &c.AffectedBlinds)
	_ = json.Unmarshal([]byte(wells), &c.AffectedWells)
	c.Active = active == 1
	return c, true, nil
}

// ResolveRetest marks the active retest case resolved with a verdict.
func (r *ArbitrationRepo) ResolveRetest(ctx context.Context, taskID string, generation int, verdict string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE retest_case SET verdict=?, active=0
		WHERE task_id=? AND generation=?`, verdict, taskID, generation)
	return err
}

// PutReview records an independent review.
func (r *ArbitrationRepo) PutReview(ctx context.Context, v arbitration.IndependentReview) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO independent_review
		(task_id, person_id, generation, decision) VALUES (?,?,?,?)`,
		v.TaskID, v.PersonID, v.Generation, string(v.Decision))
	return err
}

// Reviews returns the signed reviews for a task generation.
func (r *ArbitrationRepo) Reviews(ctx context.Context, taskID string, generation int) ([]arbitration.IndependentReview, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id, person_id, generation, decision
		FROM independent_review WHERE task_id=? AND generation=? ORDER BY person_id`, taskID, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []arbitration.IndependentReview
	for rows.Next() {
		var v arbitration.IndependentReview
		if err := rows.Scan(&v.TaskID, &v.PersonID, &v.Generation, &v.Decision); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SingleWriterRepo implements arbitration.SingleWriter over final_decision,
// whose task_id primary key enforces the single-write terminal barrier.
type SingleWriterRepo struct{ db DBTX }

// NewSingleWriterRepo builds a single-writer repository.
func NewSingleWriterRepo(db DBTX) *SingleWriterRepo { return &SingleWriterRepo{db: db} }

// CommitFinal attempts the terminal write. It returns won=false if another
// decision already committed; on admit it also stores the unique credential.
func (r *SingleWriterRepo) CommitFinal(ctx context.Context, d arbitration.FinalDecision) (arbitration.IncubationCredential, bool, error) {
	_, err := r.db.ExecContext(ctx, `INSERT INTO final_decision (task_id, kind, version, evidence_digest)
		VALUES (?,?,?,?)`, d.TaskID, string(d.Kind), d.Version, d.EvidenceDigest)
	if err != nil {
		if isUniqueViolation(err) {
			return arbitration.IncubationCredential{}, false, nil
		}
		return arbitration.IncubationCredential{}, false, err
	}
	if d.Kind != arbitration.DecisionAdmit {
		return arbitration.IncubationCredential{}, true, nil
	}
	issuer := arbitration.CredentialIssuer{}
	number := issuer.Issue(d.TaskID, d.Version)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO incubation_credential (task_id, number, version)
		VALUES (?,?,?)`, d.TaskID, number, d.Version); err != nil {
		return arbitration.IncubationCredential{}, false, err
	}
	return arbitration.IncubationCredential{TaskID: d.TaskID, Number: number, Version: d.Version}, true, nil
}

// Credential returns the stored credential for an admitted task.
func (r *SingleWriterRepo) Credential(ctx context.Context, taskID string) (arbitration.IncubationCredential, error) {
	var c arbitration.IncubationCredential
	err := r.db.QueryRowContext(ctx, `SELECT task_id, number, version FROM incubation_credential WHERE task_id=?`,
		taskID).Scan(&c.TaskID, &c.Number, &c.Version)
	return c, err
}

var _ arbitration.SingleWriter = (*SingleWriterRepo)(nil)
