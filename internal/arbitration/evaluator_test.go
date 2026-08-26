package arbitration

import (
	"errors"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
)

func TestWithinInclusiveBounds(t *testing.T) {
	th := catalog.Threshold{Min: 5000, Max: 8000, InclusiveMin: true, InclusiveMax: true}
	if !Within(5000, th) || !Within(8000, th) || !Within(6000, th) {
		t.Error("inclusive bounds should accept min, max and interior")
	}
	if Within(4999, th) || Within(8001, th) {
		t.Error("values outside inclusive bounds must be rejected")
	}
}

func TestWithinExclusiveBounds(t *testing.T) {
	th := catalog.Threshold{Min: 5000, Max: 8000, InclusiveMin: false, InclusiveMax: false}
	if Within(5000, th) || Within(8000, th) {
		t.Error("exclusive bounds must reject the endpoints")
	}
	if !Within(5001, th) || !Within(7999, th) {
		t.Error("exclusive bounds must accept the interior")
	}
}

func TestEvidenceClosureComplete(t *testing.T) {
	c := EvidenceClosure{
		CandlingComplete: true, SwabSealed: true, CultureComplete: true,
		RapidTestComplete: true, PhysicochemicalOK: true, RetestsResolved: true,
	}
	if !c.Complete() {
		t.Error("fully closed evidence should be complete")
	}
	c.RetestsResolved = false
	if c.Complete() {
		t.Error("unresolved retest must block completeness")
	}
}

func TestReviewPolicyRejectsReceiver(t *testing.T) {
	p := NewReviewPolicy([]string{"recv-1"}, nil)
	if !errors.Is(p.Validate("recv-1"), ErrReviewerIsReceiver) {
		t.Error("receiver should be rejected as reviewer")
	}
}

func TestReviewPolicyRejectsDuplicate(t *testing.T) {
	p := NewReviewPolicy(nil, []string{"rev-1"})
	if !errors.Is(p.Validate("rev-1"), ErrDuplicateReviewer) {
		t.Error("duplicate reviewer should be rejected")
	}
}

func TestCredentialDeterministic(t *testing.T) {
	issuer := CredentialIssuer{}
	a := issuer.Issue("task-1", 7)
	b := issuer.Issue("task-1", 7)
	if a != b {
		t.Errorf("credential should be deterministic: %q != %q", a, b)
	}
	if issuer.Issue("task-2", 7) == a {
		t.Error("distinct tasks must mint distinct credentials")
	}
	if !issuer.Verify("task-1", 7, a) {
		t.Error("verify should accept the issued credential")
	}
}

func TestWithinColonyUpperBound(t *testing.T) {
	th := catalog.Threshold{Min: 0, Max: 1000, InclusiveMin: true, InclusiveMax: true}
	if Within(1001, th) {
		t.Error("colony 1001 should exceed the max threshold")
	}
	if !Within(1000, th) || !Within(0, th) {
		t.Error("boundary colony values should be accepted")
	}
}
