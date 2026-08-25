package arbitration

import (
	"errors"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
)

// Review policy errors.
var (
	ErrReviewerNotQualified = errors.New("arbitration: reviewer not qualified")
	ErrReviewerIsReceiver   = errors.New("arbitration: reviewer overlaps receiver")
	ErrDuplicateReviewer    = errors.New("arbitration: reviewer already signed")
)

// ReviewPolicy enforces the independent-review rules from domain rule 8: the
// two reviewers must be distinct, each must hold the reviewer qualification,
// and neither may belong to the task's receiving pair.
type ReviewPolicy struct {
	ReviewerRole   catalog.Role
	Receivers      []string
	ExistingReview []string
}

// NewReviewPolicy builds the policy for a task's receivers and prior signers.
func NewReviewPolicy(receivers, existing []string) ReviewPolicy {
	return ReviewPolicy{
		ReviewerRole:   catalog.RoleReviewer,
		Receivers:      append([]string(nil), receivers...),
		ExistingReview: append([]string(nil), existing...),
	}
}

// Validate checks a candidate reviewer id against every rule.
func (p ReviewPolicy) Validate(personID string) error {
	for _, r := range p.Receivers {
		if r == personID {
			return ErrReviewerIsReceiver
		}
	}
	for _, s := range p.ExistingReview {
		if s == personID {
			return ErrDuplicateReviewer
		}
	}
	return nil
}

// Signed returns the updated list of reviewers including personID.
func (p ReviewPolicy) Signed(personID string) []string {
	out := append([]string(nil), p.ExistingReview...)
	return append(out, personID)
}
