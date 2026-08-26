package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

// memReader is a minimal in-memory catalog.Reader for validation tests.
type memReader struct {
	houses   map[string]CatalogHouse
	shifts   map[string]CollectionShift
	fums     map[string]FumigationBatch
	quals    map[string]PersonQualification
	rulesets map[int]RuleSetVersion
}

func newMemReader() *memReader {
	t0 := time.Now().Add(-time.Hour)
	t1 := time.Now().Add(time.Hour)
	return &memReader{
		houses: map[string]CatalogHouse{"h1": {ID: "h1", Code: "P1", ValidFrom: t0, ValidTo: t1}},
		shifts: map[string]CollectionShift{"s1": {ID: "s1", HouseID: "h1", ValidFrom: t0, ValidTo: t1}},
		fums:   map[string]FumigationBatch{"f1": {ID: "f1", Digest: "d1", Version: 1}},
		quals: map[string]PersonQualification{
			"r1": {PersonID: "r1", Roles: []Role{RoleReceiver}, ValidFrom: t0, ValidTo: t1},
		},
		rulesets: map[int]RuleSetVersion{1: {Version: 1}},
	}
}

func (m *memReader) House(ctx context.Context, id string) (CatalogHouse, error) {
	if h, ok := m.houses[id]; ok {
		return h, nil
	}
	return CatalogHouse{}, errors.New("not found")
}
func (m *memReader) Shift(ctx context.Context, id string) (CollectionShift, error) {
	if s, ok := m.shifts[id]; ok {
		return s, nil
	}
	return CollectionShift{}, errors.New("not found")
}
func (m *memReader) Fumigation(ctx context.Context, id string) (FumigationBatch, error) {
	if f, ok := m.fums[id]; ok {
		return f, nil
	}
	return FumigationBatch{}, errors.New("not found")
}
func (m *memReader) Slot(ctx context.Context, id string) (IncubatorSlot, error) {
	return IncubatorSlot{}, errors.New("not found")
}
func (m *memReader) Window(ctx context.Context, id string) (CandlingWindow, error) {
	return CandlingWindow{}, errors.New("not found")
}
func (m *memReader) Qualification(ctx context.Context, personID string) (PersonQualification, error) {
	if q, ok := m.quals[personID]; ok {
		return q, nil
	}
	return PersonQualification{}, errors.New("not found")
}
func (m *memReader) RuleSet(ctx context.Context, version int) (RuleSetVersion, error) {
	if r, ok := m.rulesets[version]; ok {
		return r, nil
	}
	return RuleSetVersion{}, errors.New("not found")
}
func (m *memReader) Device(ctx context.Context, id string) (Device, error) {
	return Device{}, errors.New("not found")
}

func TestValidateSourceOK(t *testing.T) {
	r := newMemReader()
	if _, _, err := ValidateSource(context.Background(), r, "h1", "s1"); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
}

func TestValidateSourceShiftHouseMismatch(t *testing.T) {
	r := newMemReader()
	r.shifts["s2"] = CollectionShift{ID: "s2", HouseID: "h2"}
	if _, _, err := ValidateSource(context.Background(), r, "h1", "s2"); !errors.Is(err, ErrShiftHouseMismatch) {
		t.Fatalf("mismatch = %v, want ErrShiftHouseMismatch", err)
	}
}

func TestValidateSourceHouseNotFound(t *testing.T) {
	r := newMemReader()
	if _, _, err := ValidateSource(context.Background(), r, "missing", "s1"); !errors.Is(err, ErrHouseNotFound) {
		t.Fatalf("missing house = %v, want ErrHouseNotFound", err)
	}
}

func TestValidateFumigationStale(t *testing.T) {
	r := newMemReader()
	if _, err := ValidateFumigation(context.Background(), r, "f1", "stale"); !errors.Is(err, ErrStaleFumigation) {
		t.Fatalf("stale digest = %v, want ErrStaleFumigation", err)
	}
}

func TestValidateRole(t *testing.T) {
	r := newMemReader()
	if err := ValidateRole(context.Background(), r, "r1", RoleReceiver); err != nil {
		t.Fatalf("receiver role should pass: %v", err)
	}
	if err := ValidateRole(context.Background(), r, "r1", RoleReviewer); !errors.Is(err, ErrQualification) {
		t.Fatalf("non-held role = %v, want ErrQualification", err)
	}
	if err := ValidateRole(context.Background(), r, "missing", RoleReceiver); !errors.Is(err, ErrPersonNotQualified) {
		t.Fatalf("missing person = %v, want ErrPersonNotQualified", err)
	}
}
