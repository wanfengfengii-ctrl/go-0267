package service

import (
	"context"
	"errors"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func lockRequest() LockTaskRequest {
	return LockTaskRequest{
		OperationID:       "op-lock",
		Generation:        1,
		HouseID:           "house-1",
		ShiftID:           "shift-1",
		FumigationBatchID: "fum-1",
		FumigationDigest:  "fum-digest-0001",
		RuleSetVersion:    1,
		BatchNo:           "batch-001",
		IncubatorSlotID:   "slot-1",
		CandlingWindowID:  "window-1",
		Seals:             []SealSpec{{SealNo: "seal-1", Positions: []int{1, 2, 3}}},
		BlindCodes:        []string{"blind-1", "blind-2"},
		CultureWells:      []string{"cw-1"},
		RapidWells:        []string{"rw-1"},
	}
}

func createAndLock(t *testing.T, s *Service) (string, CommandResult) {
	t.Helper()
	ctx := context.Background()
	created, err := s.CreateTask(ctx, CreateTaskRequest{OperationID: "op-create", Generation: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req := lockRequest()
	locked, err := s.LockTask(ctx, created.TaskID, req)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if locked.Status != string(task.StatusPendingReceipt) {
		t.Fatalf("after lock status = %s, want pending_receipt", locked.Status)
	}
	return created.TaskID, locked
}

func TestLockFreezesSnapshotAndLeases(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	view, err := s.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if view.BatchNo != "batch-001" || view.HouseID != "house-1" || view.ShiftID != "shift-1" {
		t.Fatalf("snapshot mismatch: %+v", view)
	}
	leases, err := s.GetLeases(ctx, id)
	if err != nil {
		t.Fatalf("get leases: %v", err)
	}
	if len(leases) != 8 { // batch, slot, window, 1 seal, 2 blinds, 1 culture, 1 rapid
		t.Fatalf("lease count = %d, want 8", len(leases))
	}
}

func TestLockStaleFumigationRejected(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	created, err := s.CreateTask(ctx, CreateTaskRequest{OperationID: "op-create", Generation: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req := lockRequest()
	req.FumigationDigest = "stale-digest"
	if _, err := s.LockTask(ctx, created.TaskID, req); err == nil {
		t.Fatal("stale fumigation digest should be rejected")
	}
}

func TestFullFlowToAdmit(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	// Dual receipt then start.
	if _, err := s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: "r1", Generation: 1, PersonID: "recv-1"}); err != nil {
		t.Fatalf("receipt 1: %v", err)
	}
	if _, err := s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: "r2", Generation: 1, PersonID: "recv-2"}); err != nil {
		t.Fatalf("receipt 2: %v", err)
	}
	if _, err := s.Start(ctx, id, StartRequest{OperationID: "start", Generation: 1}); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Candling full coverage.
	entries := []CandlingEntryInput{
		{SealNo: "seal-1", Position: 1, Category: "fertile"},
		{SealNo: "seal-1", Position: 2, Category: "fertile"},
		{SealNo: "seal-1", Position: 3, Category: "infertile"},
	}
	if _, err := s.SubmitCandling(ctx, id, SubmitCandlingRequest{OperationID: "cand", Generation: 1, Entries: entries}); err != nil {
		t.Fatalf("candling: %v", err)
	}

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

	if _, err := s.RevealBlind(ctx, id, RevealBlindRequest{OperationID: "blind", Generation: 1, Codes: []string{"blind-1", "blind-2"}}); err != nil {
		t.Fatalf("reveal blind: %v", err)
	}
	if _, err := s.AddReview(ctx, id, AddReviewRequest{OperationID: "rev1", Generation: 1, PersonID: "rev-1", Decision: "pass"}); err != nil {
		t.Fatalf("review 1: %v", err)
	}
	if _, err := s.AddReview(ctx, id, AddReviewRequest{OperationID: "rev2", Generation: 1, PersonID: "rev-2", Decision: "pass"}); err != nil {
		t.Fatalf("review 2: %v", err)
	}

	res, err := s.FinalDecision(ctx, id, FinalDecisionRequest{OperationID: "final", Generation: 1, Kind: "admit"})
	if err != nil {
		t.Fatalf("final admit: %v", err)
	}
	if res.Status != string(task.StatusAdmitted) {
		t.Fatalf("final status = %s, want admitted", res.Status)
	}
	cred, err := s.GetCredential(ctx, id)
	if err != nil {
		t.Fatalf("credential: %v", err)
	}
	if cred.Number == "" {
		t.Fatal("credential number empty")
	}
}

func TestTerminalRejectsLateWrites(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)
	if _, err := s.FinalDecision(ctx, id, FinalDecisionRequest{OperationID: "cancel", Generation: 1, Kind: "cancel", PersonID: "auth-1"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: "late", Generation: 1, PersonID: "recv-1"}); !errors.Is(err, ErrTerminal) {
		t.Fatalf("late write after terminal = %v, want ErrTerminal", err)
	}
}

func TestOperationIdempotency(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	req := AddReceiptRequest{OperationID: "r1", Generation: 1, PersonID: "recv-1"}
	if _, err := s.AddReceipt(ctx, id, req); err != nil {
		t.Fatalf("first receipt: %v", err)
	}
	if _, err := s.AddReceipt(ctx, id, req); err != nil {
		t.Fatalf("replay receipt: %v", err)
	}
	conflict := req
	conflict.PersonID = "recv-2"
	if _, err := s.AddReceipt(ctx, id, conflict); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("conflict = %v, want ErrOperationConflict", err)
	}
}

func TestReviewerCannotOverlapReceiver(t *testing.T) {
	// Assert the review policy layer directly (full flow covered elsewhere).
	policy := arbitration.NewReviewPolicy([]string{"recv-1"}, []string{"rev-1"})
	if err := policy.Validate("recv-1"); err == nil {
		t.Fatal("receiver should be rejected as reviewer")
	}
	if err := policy.Validate("rev-1"); err == nil {
		t.Fatal("duplicate reviewer should be rejected")
	}
	if err := policy.Validate("rev-2"); err != nil {
		t.Fatalf("distinct reviewer should pass: %v", err)
	}
}
