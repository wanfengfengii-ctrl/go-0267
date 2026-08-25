package evidence

import (
	"errors"
	"testing"
)

func entries(positions ...int) []CandlingEntry {
	out := make([]CandlingEntry, 0, len(positions))
	for _, p := range positions {
		out = append(out, CandlingEntry{SealNo: "s", Position: p, Category: CategoryFertile})
	}
	return out
}

func TestValidateCoverageComplete(t *testing.T) {
	if err := ValidateCoverage(entries(1, 2, 3), 3); err != nil {
		t.Fatalf("complete coverage failed: %v", err)
	}
}

func TestValidateCoverageMissing(t *testing.T) {
	if err := ValidateCoverage(entries(1, 2), 3); !errors.Is(err, ErrMissingPosition) {
		t.Fatalf("missing position = %v, want ErrMissingPosition", err)
	}
}

func TestValidateCoverageDuplicate(t *testing.T) {
	if err := ValidateCoverage(entries(1, 1, 2), 3); !errors.Is(err, ErrDuplicatePosition) {
		t.Fatalf("duplicate position = %v, want ErrDuplicatePosition", err)
	}
}

func TestCategoryCountsConservation(t *testing.T) {
	in := []CandlingEntry{
		{SealNo: "s", Position: 1, Category: CategoryFertile},
		{SealNo: "s", Position: 2, Category: CategoryInfertile},
		{SealNo: "s", Position: 3, Category: CategoryFertile},
	}
	counts := CategoryCounts(in)
	if counts[CategoryFertile] != 2 || counts[CategoryInfertile] != 1 {
		t.Fatalf("counts = %v", counts)
	}
	sum := 0
	for _, c := range counts {
		sum += c
	}
	if sum != len(in) {
		t.Fatalf("primary classification sum = %d, want %d", sum, len(in))
	}
}

func TestValidateEntriesDefectMismatch(t *testing.T) {
	in := []CandlingEntry{{SealNo: "s", Position: 1, Category: CategoryFertile, Defects: []Defect{DefectCrack}}}
	if err := ValidateEntries(in); !errors.Is(err, ErrDefectMismatch) {
		t.Fatalf("defect mismatch = %v, want ErrDefectMismatch", err)
	}
}

func TestValidateEntriesCompatible(t *testing.T) {
	in := []CandlingEntry{{SealNo: "s", Position: 1, Category: CategoryCracked, Defects: []Defect{DefectCrack}}}
	if err := ValidateEntries(in); err != nil {
		t.Fatalf("compatible entry failed: %v", err)
	}
}

func TestParseFixedThresholdBoundary(t *testing.T) {
	// Boundary values used by the acceptance fixed-point scenario.
	cases := []struct {
		in        string
		precision int
		want      int64
	}{
		{"50.00", 2, 5000},
		{"80.00", 2, 8000},
		{"0", 0, 0},
		{"1000", 0, 1000},
		{"40.0", 1, 400},
	}
	for _, c := range cases {
		got, err := ParseFixed(c.in, c.precision)
		if err != nil {
			t.Errorf("ParseFixed(%q,%d): %v", c.in, c.precision, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseFixed(%q,%d) = %d, want %d", c.in, c.precision, got, c.want)
		}
	}
}
