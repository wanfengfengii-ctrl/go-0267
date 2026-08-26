package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
)

// CatalogRepo implements catalog.Reader over the SQL database.
type CatalogRepo struct{ db DBTX }

// NewCatalogRepo builds a catalog reader over the given query surface.
func NewCatalogRepo(db DBTX) *CatalogRepo { return &CatalogRepo{db: db} }

// seedCatalog populates the reference catalog when it is empty, providing a
// deterministic directory for the smoke test and offline acceptance runs.
func (s *Store) seedCatalog() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM catalog_house`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	t0 := int64(0)          // valid-from "forever"
	t1 := int64(4102444800) // year 2100
	if _, err := tx.Exec(`INSERT INTO catalog_house (id, code, name, valid_from, valid_to) VALUES
		('house-1','P1','父母代鸡舍一号',?,?),
		('house-2','P2','父母代鸡舍二号',?,?)`, t0, t1, t0, t1); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO collection_shift (id, house_id, code, valid_from, valid_to) VALUES
		('shift-1','house-1','早班',?,?),
		('shift-2','house-2','晚班',?,?)`, t0, t1, t0, t1); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO fumigation_batch (id, digest, version) VALUES
		('fum-1','fum-digest-0001',1),
		('fum-2','fum-digest-0002',2)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO incubator_slot (id, code, valid_from, valid_to) VALUES
		('slot-1','INC-01',?,?),
		('slot-2','INC-02',?,?)`, t0, t1, t0, t1); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO candling_window (id, code) VALUES
		('window-1','CW-01'),
		('window-2','CW-02')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO device (id, kind, code) VALUES (?,?,?), (?,?,?), (?,?,?), (?,?,?)`,
		"dev-candler", string(catalog.DeviceCandler), "CL-1",
		"dev-culture", string(catalog.DeviceCultureBox), "CB-1",
		"dev-reader", string(catalog.DevicePlateReader), "PR-1",
		"dev-scale", string(catalog.DeviceScale), "SC-1"); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO person_qualification (person_id, role, valid_from, valid_to) VALUES
		('recv-1','receiver',?,?),
		('recv-2','receiver',?,?),
		('rev-1','reviewer',?,?),
		('rev-2','reviewer',?,?),
		('auth-1','authorizer',?,?),
		('dual-1','receiver',?,?),
		('dual-1','reviewer',?,?)`, t0, t1, t0, t1, t0, t1, t0, t1, t0, t1, t0, t1, t0, t1); err != nil {
		return err
	}
	precisions, _ := json.Marshal(map[string]int{
		string(catalog.EvidenceEggWeight):   2,
		string(catalog.EvidenceAirCell):     2,
		string(catalog.EvidenceCleanliness): 0,
		string(catalog.EvidenceColonyCount): 0,
		string(catalog.EvidenceCtValue):     1,
		string(catalog.EvidenceFumigation):  2,
	})
	thresholds, _ := json.Marshal(map[string]catalog.Threshold{
		string(catalog.EvidenceEggWeight):   {Min: 5000, Max: 8000, InclusiveMin: true, InclusiveMax: true},
		string(catalog.EvidenceAirCell):     {Min: 100, Max: 500, InclusiveMin: true, InclusiveMax: true},
		string(catalog.EvidenceCleanliness): {Min: 0, Max: 3, InclusiveMin: true, InclusiveMax: true},
		string(catalog.EvidenceColonyCount): {Min: 0, Max: 1000, InclusiveMin: true, InclusiveMax: true},
		string(catalog.EvidenceCtValue):     {Min: 0, Max: 400, InclusiveMin: true, InclusiveMax: true},
		string(catalog.EvidenceFumigation):  {Min: 0, Max: 100, InclusiveMin: true, InclusiveMax: true},
	})
	if _, err := tx.Exec(`INSERT INTO ruleset_version (version, precisions, thresholds) VALUES (1,?,?)`,
		string(precisions), string(thresholds)); err != nil {
		return err
	}
	return tx.Commit()
}

// House fetches a catalog house.
func (r *CatalogRepo) House(ctx context.Context, id string) (catalog.CatalogHouse, error) {
	var h catalog.CatalogHouse
	var f, t int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code, name, valid_from, valid_to FROM catalog_house WHERE id=?`, id).
		Scan(&h.ID, &h.Code, &h.Name, &f, &t)
	if err != nil {
		return h, err
	}
	h.ValidFrom, h.ValidTo = time.Unix(f, 0), time.Unix(t, 0)
	return h, nil
}

// Shift fetches a collection shift.
func (r *CatalogRepo) Shift(ctx context.Context, id string) (catalog.CollectionShift, error) {
	var s catalog.CollectionShift
	var f, t int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, house_id, code, valid_from, valid_to FROM collection_shift WHERE id=?`, id).
		Scan(&s.ID, &s.HouseID, &s.Code, &f, &t)
	if err != nil {
		return s, err
	}
	s.ValidFrom, s.ValidTo = time.Unix(f, 0), time.Unix(t, 0)
	return s, nil
}

// Fumigation fetches a fumigation batch.
func (r *CatalogRepo) Fumigation(ctx context.Context, id string) (catalog.FumigationBatch, error) {
	var b catalog.FumigationBatch
	err := r.db.QueryRowContext(ctx,
		`SELECT id, digest, version FROM fumigation_batch WHERE id=?`, id).
		Scan(&b.ID, &b.Digest, &b.Version)
	return b, err
}

// Slot fetches an incubator slot.
func (r *CatalogRepo) Slot(ctx context.Context, id string) (catalog.IncubatorSlot, error) {
	var s catalog.IncubatorSlot
	var f, t int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code, valid_from, valid_to FROM incubator_slot WHERE id=?`, id).
		Scan(&s.ID, &s.Code, &f, &t)
	if err != nil {
		return s, err
	}
	s.ValidFrom, s.ValidTo = time.Unix(f, 0), time.Unix(t, 0)
	return s, nil
}

// Window fetches a candling window.
func (r *CatalogRepo) Window(ctx context.Context, id string) (catalog.CandlingWindow, error) {
	var w catalog.CandlingWindow
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code FROM candling_window WHERE id=?`, id).Scan(&w.ID, &w.Code)
	return w, err
}

// Qualification fetches a person's qualification record.
func (r *CatalogRepo) Qualification(ctx context.Context, personID string) (catalog.PersonQualification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT person_id, role, valid_from, valid_to FROM person_qualification WHERE person_id=?`, personID)
	if err != nil {
		return catalog.PersonQualification{}, err
	}
	defer rows.Close()
	q := catalog.PersonQualification{PersonID: personID}
	for rows.Next() {
		var role catalog.Role
		var f, t int64
		if err := rows.Scan(&q.PersonID, &role, &f, &t); err != nil {
			return q, err
		}
		q.Roles = append(q.Roles, role)
		if q.ValidFrom.IsZero() || f < q.ValidFrom.Unix() {
			q.ValidFrom = time.Unix(f, 0)
		}
		q.ValidTo = time.Unix(t, 0)
	}
	if len(q.Roles) == 0 {
		return q, sql.ErrNoRows
	}
	return q, nil
}

// RuleSet fetches a versioned rule set.
func (r *CatalogRepo) RuleSet(ctx context.Context, version int) (catalog.RuleSetVersion, error) {
	var v catalog.RuleSetVersion
	var pJSON, tJSON string
	err := r.db.QueryRowContext(ctx,
		`SELECT version, precisions, thresholds FROM ruleset_version WHERE version=?`, version).
		Scan(&v.Version, &pJSON, &tJSON)
	if err != nil {
		return v, err
	}
	prec := map[string]int{}
	if err := json.Unmarshal([]byte(pJSON), &prec); err != nil {
		return v, err
	}
	thr := map[string]catalog.Threshold{}
	if err := json.Unmarshal([]byte(tJSON), &thr); err != nil {
		return v, err
	}
	v.Precisions = make(map[catalog.EvidenceKind]int, len(prec))
	for k, val := range prec {
		v.Precisions[catalog.EvidenceKind(k)] = val
	}
	v.Thresholds = make(map[catalog.EvidenceKind]catalog.Threshold, len(thr))
	for k, val := range thr {
		v.Thresholds[catalog.EvidenceKind(k)] = val
	}
	return v, nil
}

// Device fetches a catalog device by id.
func (r *CatalogRepo) Device(ctx context.Context, id string) (catalog.Device, error) {
	var d catalog.Device
	err := r.db.QueryRowContext(ctx, `SELECT id, kind, code FROM device WHERE id=?`, id).
		Scan(&d.ID, &d.Kind, &d.Code)
	return d, err
}
