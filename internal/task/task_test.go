package task

import "testing"

func TestTaskStatusValidity(t *testing.T) {
	all := []TaskStatus{
		StatusPendingLock, StatusPendingReceipt, StatusResourcesOccupied,
		StatusCandling, StatusSwabCulture, StatusRapidTest, StatusPhysicochemical,
		StatusPendingReview, StatusAdmittable, StatusAdmitted, StatusIsolated,
		StatusCancelled,
	}
	for _, s := range all {
		if !s.Valid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if TaskStatus("bogus").Valid() {
		t.Error("unknown status must not be valid")
	}
}

func TestTaskStatusTerminal(t *testing.T) {
	terminals := map[TaskStatus]bool{
		StatusAdmitted:  true,
		StatusIsolated:  true,
		StatusCancelled: true,
	}
	for _, s := range []TaskStatus{
		StatusPendingLock, StatusPendingReceipt, StatusResourcesOccupied,
		StatusCandling, StatusSwabCulture, StatusRapidTest, StatusPhysicochemical,
		StatusPendingReview, StatusAdmittable, StatusAdmitted, StatusIsolated,
		StatusCancelled,
	} {
		if got := s.Terminal(); got != terminals[s] {
			t.Errorf("Terminal(%q) = %v, want %v", s, got, terminals[s])
		}
	}
}

func TestLinearProgression(t *testing.T) {
	path := []TaskStatus{
		StatusPendingLock, StatusPendingReceipt, StatusResourcesOccupied,
		StatusCandling, StatusSwabCulture, StatusRapidTest, StatusPhysicochemical,
		StatusPendingReview, StatusAdmittable,
	}
	for i := 0; i < len(path)-1; i++ {
		if !path[i].CanTransitionTo(path[i+1]) {
			t.Errorf("%q should transition to %q", path[i], path[i+1])
		}
	}
}

func TestSkipForbidden(t *testing.T) {
	if StatusPendingLock.CanTransitionTo(StatusCandling) {
		t.Error("must not skip pending_receipt and resources_occupied")
	}
	if StatusCandling.CanTransitionTo(StatusPendingReview) {
		t.Error("must not skip collection stages")
	}
}

func TestTerminalBlocksAllTransitions(t *testing.T) {
	for _, terminal := range []TaskStatus{StatusAdmitted, StatusIsolated, StatusCancelled} {
		for _, next := range []TaskStatus{StatusAdmitted, StatusIsolated, StatusCancelled, StatusPendingLock} {
			if terminal.CanTransitionTo(next) {
				t.Errorf("terminal %q must not transition to %q", terminal, next)
			}
		}
	}
}

func TestCancelFromNonTerminal(t *testing.T) {
	for _, s := range []TaskStatus{
		StatusPendingLock, StatusPendingReceipt, StatusResourcesOccupied,
		StatusCandling, StatusSwabCulture, StatusRapidTest, StatusPhysicochemical,
		StatusPendingReview, StatusAdmittable,
	} {
		if !s.CanTransitionTo(StatusCancelled) {
			t.Errorf("%q should allow cancellation", s)
		}
	}
}

func TestAdmitAndIsolateOnlyFromAdmittable(t *testing.T) {
	if !StatusAdmittable.CanTransitionTo(StatusAdmitted) {
		t.Error("admittable should allow admit")
	}
	if !StatusAdmittable.CanTransitionTo(StatusIsolated) {
		t.Error("admittable should allow isolate")
	}
	if StatusPendingReview.CanTransitionTo(StatusAdmitted) {
		t.Error("only admittable may admit")
	}
	if StatusPendingReview.CanTransitionTo(StatusIsolated) {
		t.Error("only admittable may isolate")
	}
}
