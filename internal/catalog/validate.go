package catalog

import (
	"context"
	"errors"
	"time"
)

// Validation errors map to stable business codes at the API layer.
var (
	ErrHouseNotFound      = errors.New("catalog: house not found")
	ErrShiftNotFound      = errors.New("catalog: shift not found")
	ErrShiftHouseMismatch = errors.New("catalog: shift does not belong to house")
	ErrFumigationNotFound = errors.New("catalog: fumigation batch not found")
	ErrStaleFumigation    = errors.New("catalog: stale fumigation digest")
	ErrQualification      = errors.New("catalog: person lacks required qualification")
	ErrRuleSetNotFound    = errors.New("catalog: ruleset version not found")
	ErrPersonNotQualified = errors.New("catalog: person not qualified")
)

// now reports the reference instant used for validity checks. It is a var so
// tests may pin it without introducing a wall-clock dependency.
var now = time.Now

// ValidateSource checks that a house and its collection shift both exist and
// are currently effective, and that the shift belongs to the house. A locked
// task may only proceed when both relations hold.
func ValidateSource(ctx context.Context, r Reader, houseID, shiftID string) (CatalogHouse, CollectionShift, error) {
	house, err := r.House(ctx, houseID)
	if err != nil {
		return CatalogHouse{}, CollectionShift{}, ErrHouseNotFound
	}
	shift, err := r.Shift(ctx, shiftID)
	if err != nil {
		return CatalogHouse{}, CollectionShift{}, ErrShiftNotFound
	}
	if shift.HouseID != house.ID {
		return CatalogHouse{}, CollectionShift{}, ErrShiftHouseMismatch
	}
	t := now()
	if !effective(house.ValidFrom, house.ValidTo, t) {
		return CatalogHouse{}, CollectionShift{}, ErrHouseNotFound
	}
	if !effective(shift.ValidFrom, shift.ValidTo, t) {
		return CatalogHouse{}, CollectionShift{}, ErrShiftNotFound
	}
	return house, shift, nil
}

// ValidateFumigation checks that the fumigation batch exists and that the
// submitted digest matches the current version, rejecting stale summaries.
func ValidateFumigation(ctx context.Context, r Reader, id, digest string) (FumigationBatch, error) {
	batch, err := r.Fumigation(ctx, id)
	if err != nil {
		return FumigationBatch{}, ErrFumigationNotFound
	}
	if batch.Digest != digest {
		return FumigationBatch{}, ErrStaleFumigation
	}
	return batch, nil
}

// ValidateRole checks a person currently holds the required role.
func ValidateRole(ctx context.Context, r Reader, personID string, role Role) error {
	q, err := r.Qualification(ctx, personID)
	if err != nil {
		return ErrPersonNotQualified
	}
	t := now()
	if !effective(q.ValidFrom, q.ValidTo, t) {
		return ErrQualification
	}
	for _, got := range q.Roles {
		if got == role {
			return nil
		}
	}
	return ErrQualification
}

func effective(from, to, t time.Time) bool {
	return (from.IsZero() || !t.Before(from)) && (to.IsZero() || !t.After(to))
}
