package evidence

import (
	"errors"
	"fmt"
	"sort"
)

// Candling matrix validation errors. These map to stable business codes.
var (
	ErrDuplicatePosition = errors.New("evidence: duplicate position in matrix")
	ErrMissingPosition   = errors.New("evidence: missing position in matrix")
	ErrInvalidCategory   = errors.New("evidence: invalid candling category")
	ErrInvalidDefect     = errors.New("evidence: invalid defect marker")
	ErrDefectMismatch    = errors.New("evidence: defect marker inconsistent with category")
)

// ValidateCoverage checks that the submitted entries exactly cover positions
// 1..totalPositions with no duplicates and no gaps. A valid coverage matrix is
// a precondition for any closed candling stage.
func ValidateCoverage(entries []CandlingEntry, totalPositions int) error {
	if len(entries) != totalPositions {
		if len(entries) < totalPositions {
			return ErrMissingPosition
		}
		return ErrDuplicatePosition
	}
	seen := make(map[int]bool, len(entries))
	for _, e := range entries {
		if e.Position < 1 || e.Position > totalPositions {
			return fmt.Errorf("%w: position %d out of range", ErrInvalidCategory, e.Position)
		}
		if seen[e.Position] {
			return fmt.Errorf("%w: position %d", ErrDuplicatePosition, e.Position)
		}
		seen[e.Position] = true
	}
	for p := 1; p <= totalPositions; p++ {
		if !seen[p] {
			return fmt.Errorf("%w: position %d", ErrMissingPosition, p)
		}
	}
	return nil
}

// CategoryCounts tallies the primary classification counts. The sum of all
// counts must equal the number of submitted positions (integer conservation).
func CategoryCounts(entries []CandlingEntry) map[CandlingCategory]int {
	out := make(map[CandlingCategory]int, 5)
	for _, e := range entries {
		out[e.Category]++
	}
	return out
}

// ValidateEntries checks category/defect enums and the defect-to-category
// consistency rule: additive markers may only appear where the primary
// classification already reports the defect (crack, blood spot, contamination).
func ValidateEntries(entries []CandlingEntry) error {
	for _, e := range entries {
		if !e.Category.Valid() {
			return fmt.Errorf("%w: %q", ErrInvalidCategory, e.Category)
		}
		for _, d := range e.Defects {
			if !d.Valid() {
				return fmt.Errorf("%w: %q", ErrInvalidDefect, d)
			}
			if !defectCompatible(e.Category, d) {
				return fmt.Errorf("%w: category %q with defect %q", ErrDefectMismatch, e.Category, d)
			}
		}
	}
	return nil
}

func defectCompatible(c CandlingCategory, d Defect) bool {
	switch d {
	case DefectCrack:
		return c == CategoryCracked
	case DefectBloodSpot:
		return c == CategoryBloodSpot
	case DefectContaminate:
		return c == CategoryContaminated
	default:
		return false
	}
}

// SortedPositions returns the distinct positions in ascending order.
func SortedPositions(entries []CandlingEntry) []int {
	out := make([]int, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Position)
	}
	sort.Ints(out)
	return out
}
