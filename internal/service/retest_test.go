package service

import (
	"context"
	"errors"
	"testing"
)

func TestSingleActiveRetestPerGeneration(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	req := CreateRetestRequest{
		OperationID: "ret1", Generation: 1, Trigger: "suspect_positive",
		AffectedSeals: []string{"seal-1"}, AffectedPositions: []int{2},
		AffectedBlinds: []string{"blind-1"}, AffectedWells: []string{"cw-1"},
	}
	if _, err := s.CreateRetest(ctx, id, req); err != nil {
		t.Fatalf("create retest: %v", err)
	}
	// A second active retest for the same generation must be rejected.
	req.OperationID = "ret2"
	if _, err := s.CreateRetest(ctx, id, req); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second retest = %v, want ErrInvalidState", err)
	}

	// Append evidence and resolve the case.
	if _, err := s.AddRetestEvidence(ctx, id, RetestEvidenceRequest{
		OperationID: "ev1", Generation: 1, Kind: "colony_count", Value: 12, Verdict: "contaminated",
	}); err != nil {
		t.Fatalf("add retest evidence: %v", err)
	}

	ev, err := s.GetEvidence(ctx, id)
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if ev.Retest != nil {
		t.Fatal("retest should be resolved and absent from the active view")
	}
}

func TestRetestLateGenerationIgnored(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	if _, err := s.CreateRetest(ctx, id, CreateRetestRequest{
		OperationID: "ret1", Generation: 1, Trigger: "culture_pollution",
		AffectedWells: []string{"cw-1"},
	}); err != nil {
		t.Fatalf("create retest: %v", err)
	}
	// Evidence for a stale generation is rejected (no active retest there).
	if _, err := s.AddRetestEvidence(ctx, id, RetestEvidenceRequest{
		OperationID: "stale", Generation: 2, Kind: "colony_count", Value: 3,
	}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale generation evidence = %v, want ErrStaleGeneration", err)
	}
}

func TestReviewRequiresTwoDistinctPass(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)
	advanceToCulture(t, s, id)
	if _, err := s.SealSwab(ctx, id, SealSwabRequest{OperationID: "swab", Generation: 1, SealNo: "seal-1"}); err != nil {
		t.Fatalf("seal swab: %v", err)
	}
	if _, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{OperationID: "cult", Generation: 1, Well: "cw-1", DeviceID: "dev-culture"}); err != nil {
		t.Fatalf("culture: %v", err)
	}
	if _, err := s.SubmitRapidTest(ctx, id, RapidTestRequest{OperationID: "rapid", Generation: 1, Well: "rw-1", DeviceID: "dev-reader"}); err != nil {
		t.Fatalf("rapid: %v", err)
	}
	for _, k := range []string{"egg_weight", "air_cell_height", "cleanliness", "fumigation_residue"} {
		if _, err := s.SubmitPhysicochemical(ctx, id, PhysicochemicalRequest{OperationID: "phys-" + k, Generation: 1, SealNo: "seal-1", Position: 1, Kind: k, DeviceID: "dev-scale"}); err != nil {
			t.Fatalf("physicochemical %s: %v", k, err)
		}
	}

	// A person who is only a receiver is not a qualified reviewer.
	if _, err := s.AddReview(ctx, id, AddReviewRequest{OperationID: "rv", Generation: 1, PersonID: "recv-1", Decision: "pass"}); !errors.Is(err, ErrNotQualified) {
		t.Fatalf("receiver review = %v, want ErrNotQualified", err)
	}
	// A single passing review must not admit.
	if _, err := s.AddReview(ctx, id, AddReviewRequest{OperationID: "rv1", Generation: 1, PersonID: "rev-1", Decision: "pass"}); err != nil {
		t.Fatalf("review 1: %v", err)
	}
	if _, err := s.FinalDecision(ctx, id, FinalDecisionRequest{OperationID: "f", Generation: 1, Kind: "admit"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("admit with one review = %v, want ErrInvalidState", err)
	}
}
