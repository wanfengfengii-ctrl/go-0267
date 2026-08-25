package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// failingDevice returns a device that always fails with the given error.
func failingDevice(err error) evidence.DevicePort {
	return deviceFunc(func(ctx context.Context, a evidence.DeviceAttempt) (string, error) {
		return "", err
	})
}

// advanceToCulture drives a freshly locked task to the swab_culture stage.
func advanceToCulture(t *testing.T, s *Service, id string) {
	t.Helper()
	ctx := context.Background()
	for _, p := range []struct {
		op, person string
	}{
		{"r1", "recv-1"}, {"r2", "recv-2"},
	} {
		if _, err := s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: p.op, Generation: 1, PersonID: p.person}); err != nil {
			t.Fatalf("receipt: %v", err)
		}
	}
	if _, err := s.Start(ctx, id, StartRequest{OperationID: "start", Generation: 1}); err != nil {
		t.Fatalf("start: %v", err)
	}
	entries := []CandlingEntryInput{
		{SealNo: "seal-1", Position: 1, Category: "fertile"},
		{SealNo: "seal-1", Position: 2, Category: "fertile"},
		{SealNo: "seal-1", Position: 3, Category: "infertile"},
	}
	if _, err := s.SubmitCandling(ctx, id, SubmitCandlingRequest{OperationID: "cand", Generation: 1, Entries: entries}); err != nil {
		t.Fatalf("candling: %v", err)
	}
}

func TestDeviceFailureRecordsPendingRetry(t *testing.T) {
	s := newTestService(t)
	s.SetDevice("dev-culture", failingDevice(evidence.ErrDeviceTimeout))
	ctx := context.Background()
	id, _ := createAndLock(t, s)
	advanceToCulture(t, s, id)

	res, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{
		OperationID: "cult", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
	})
	if err != nil {
		t.Fatalf("culture submit: %v", err)
	}
	if !res.PendingRetry {
		t.Fatal("expected pending retry on device failure")
	}

	pending, err := s.PendingRetries(ctx, id)
	if err != nil {
		t.Fatalf("pending retries: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending attempt count = %d, want 1", len(pending))
	}
	if pending[0].Failure != evidence.FailureTimeout {
		t.Fatalf("failure = %s, want timeout", pending[0].Failure)
	}
	if pending[0].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", pending[0].Attempt)
	}

	// No culture evidence must have been fabricated.
	ev, err := s.GetEvidence(ctx, id)
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if len(ev.Culture) != 0 {
		t.Fatalf("culture evidence count = %d, want 0", len(ev.Culture))
	}
}

func TestDeviceRetrySurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hatchseal.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s := New(st)
	s.SetDevice("dev-culture", failingDevice(evidence.ErrDeviceDown))
	ctx := context.Background()
	id, _ := createAndLock(t, s)
	advanceToCulture(t, s, id)
	if _, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{
		OperationID: "cult", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
	}); err != nil {
		t.Fatalf("culture submit: %v", err)
	}
	st.Close()

	// Reopen the same file: the pending attempt must be recovered.
	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st2.Close()
	s2 := New(st2)
	pending, err := s2.PendingRetries(ctx, id)
	if err != nil {
		t.Fatalf("pending retries after restart: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending attempt count after restart = %d, want 1", len(pending))
	}
	if pending[0].Failure != evidence.FailureDown {
		t.Fatalf("recovered failure = %s, want down", pending[0].Failure)
	}
	// The task must still be open and not terminal.
	view, err := s2.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if view.Status == string(task.StatusAdmitted) || view.Status == string(task.StatusCancelled) {
		t.Fatalf("task should remain open, got %s", view.Status)
	}
}

func TestResourceConflictBetweenTasks(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	id, _ := createAndLock(t, s)

	// A second task using the same batch number must fail to lock.
	created, err := s.CreateTask(ctx, CreateTaskRequest{OperationID: "op-create-2", Generation: 1})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	req := lockRequest()
	req.OperationID = "op-lock-2"
	if _, err := s.LockTask(ctx, created.TaskID, req); err == nil {
		t.Fatal("second lock on same batch should be rejected")
	}

	// The first task must still hold all of its leases.
	leases, err := s.GetLeases(ctx, id)
	if err != nil {
		t.Fatalf("get leases: %v", err)
	}
	if len(leases) != 8 {
		t.Fatalf("first task lease count = %d, want 8", len(leases))
	}
}
