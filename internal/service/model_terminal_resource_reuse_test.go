package service

import (
	"context"
	"errors"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

func TestModel_TerminalTasksReleaseReusableResources(t *testing.T) {
	tests := []struct {
		name             string
		terminalDecision string
		finalize         func(context.Context, *testing.T, *Service, string)
		expectCredential bool
	}{
		{
			name:             "cancelled task releases slot window and test wells",
			terminalDecision: "cancel",
			finalize: func(ctx context.Context, t *testing.T, s *Service, taskID string) {
				t.Helper()
				got, err := s.FinalDecision(ctx, taskID, FinalDecisionRequest{
					OperationID: "final-cancel",
					Generation:  1,
					Kind:        "cancel",
					PersonID:    "auth-1",
				})
				if err != nil {
					t.Fatalf("cancel terminal decision: %v", err)
				}
				if got.Status != string(task.StatusCancelled) {
					t.Fatalf("terminal status = %s, want %s", got.Status, task.StatusCancelled)
				}
			},
		},
		{
			name:             "admitted task releases slot window and test wells",
			terminalDecision: "admit",
			expectCredential: true,
			finalize: func(ctx context.Context, t *testing.T, s *Service, taskID string) {
				t.Helper()
				modelReachAdmittable(ctx, t, s, taskID)
				got, err := s.FinalDecision(ctx, taskID, FinalDecisionRequest{
					OperationID: "final-admit",
					Generation:  1,
					Kind:        "admit",
				})
				if err != nil {
					t.Fatalf("admit terminal decision: %v", err)
				}
				if got.Status != string(task.StatusAdmitted) {
					t.Fatalf("terminal status = %s, want %s", got.Status, task.StatusAdmitted)
				}
			},
		},
		{
			name:             "isolated task releases slot window and test wells",
			terminalDecision: "isolate",
			finalize: func(ctx context.Context, t *testing.T, s *Service, taskID string) {
				t.Helper()
				modelReachAdmittable(ctx, t, s, taskID)
				got, err := s.FinalDecision(ctx, taskID, FinalDecisionRequest{
					OperationID: "final-isolate",
					Generation:  1,
					Kind:        "isolate",
				})
				if err != nil {
					t.Fatalf("isolate terminal decision: %v", err)
				}
				if got.Status != string(task.StatusIsolated) {
					t.Fatalf("terminal status = %s, want %s", got.Status, task.StatusIsolated)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestService(t)
			terminalTaskID, _ := createAndLock(t, s)
			tt.finalize(ctx, t, s, terminalTaskID)

			if _, err := s.AddReceipt(ctx, terminalTaskID, AddReceiptRequest{
				OperationID: "late-receipt",
				Generation:  1,
				PersonID:    "recv-1",
			}); !errors.Is(err, ErrTerminal) {
				t.Fatalf("late receipt after %s = %v, want ErrTerminal", tt.terminalDecision, err)
			}
			if _, err := s.FinalDecision(ctx, terminalTaskID, FinalDecisionRequest{
				OperationID: "late-final",
				Generation:  1,
				Kind:        "cancel",
				PersonID:    "auth-1",
			}); !errors.Is(err, ErrTerminal) {
				t.Fatalf("second terminal decision after %s = %v, want ErrTerminal", tt.terminalDecision, err)
			}
			if tt.expectCredential {
				first, err := s.GetCredential(ctx, terminalTaskID)
				if err != nil {
					t.Fatalf("credential after admit: %v", err)
				}
				second, err := s.GetCredential(ctx, terminalTaskID)
				if err != nil {
					t.Fatalf("credential reread after admit: %v", err)
				}
				if first.Number == "" || first != second {
					t.Fatalf("credential not stable: first=%+v second=%+v", first, second)
				}
			}

			reuseTaskID := modelCreateTask(ctx, t, s, "create-reuse")
			if got, err := s.LockTask(ctx, reuseTaskID, modelLockRequest("reuse")); err != nil {
				t.Fatalf("new open task reusing terminal task slot/window/wells: %v", err)
			} else if got.Status != string(task.StatusPendingReceipt) {
				t.Fatalf("reuse lock status = %s, want %s", got.Status, task.StatusPendingReceipt)
			}

			competingTaskID := modelCreateTask(ctx, t, s, "create-competing")
			if _, err := s.LockTask(ctx, competingTaskID, modelLockRequest("competing")); !errors.Is(err, resource.ErrLeaseConflict) {
				t.Fatalf("open task competing for reused slot/window/wells = %v, want ErrLeaseConflict", err)
			}
		})
	}
}

func modelReachAdmittable(ctx context.Context, t *testing.T, s *Service, taskID string) {
	t.Helper()
	if _, err := s.AddReceipt(ctx, taskID, AddReceiptRequest{OperationID: "receipt-1", Generation: 1, PersonID: "recv-1"}); err != nil {
		t.Fatalf("receipt 1: %v", err)
	}
	if _, err := s.AddReceipt(ctx, taskID, AddReceiptRequest{OperationID: "receipt-2", Generation: 1, PersonID: "recv-2"}); err != nil {
		t.Fatalf("receipt 2: %v", err)
	}
	if _, err := s.Start(ctx, taskID, StartRequest{OperationID: "start", Generation: 1}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := s.SubmitCandling(ctx, taskID, SubmitCandlingRequest{
		OperationID: "candling",
		Generation:  1,
		Entries: []CandlingEntryInput{
			{SealNo: "seal-1", Position: 1, Category: "fertile"},
			{SealNo: "seal-1", Position: 2, Category: "fertile"},
			{SealNo: "seal-1", Position: 3, Category: "infertile"},
		},
	}); err != nil {
		t.Fatalf("candling: %v", err)
	}
	if _, err := s.SealSwab(ctx, taskID, SealSwabRequest{OperationID: "swab", Generation: 1, SealNo: "seal-1"}); err != nil {
		t.Fatalf("seal swab: %v", err)
	}
	if _, err := s.SubmitCultureReading(ctx, taskID, CultureReadingRequest{OperationID: "culture", Generation: 1, Well: "cw-1", DeviceID: "dev-culture"}); err != nil {
		t.Fatalf("culture: %v", err)
	}
	if _, err := s.SubmitRapidTest(ctx, taskID, RapidTestRequest{OperationID: "rapid", Generation: 1, Well: "rw-1", DeviceID: "dev-reader"}); err != nil {
		t.Fatalf("rapid: %v", err)
	}
	for _, kind := range []string{"egg_weight", "air_cell_height", "cleanliness", "fumigation_residue"} {
		if _, err := s.SubmitPhysicochemical(ctx, taskID, PhysicochemicalRequest{OperationID: "phys-" + kind, Generation: 1, SealNo: "seal-1", Position: 1, Kind: kind, DeviceID: "dev-scale"}); err != nil {
			t.Fatalf("physicochemical %s: %v", kind, err)
		}
	}
	if _, err := s.RevealBlind(ctx, taskID, RevealBlindRequest{OperationID: "blind", Generation: 1, Codes: []string{"blind-1", "blind-2"}}); err != nil {
		t.Fatalf("reveal blind: %v", err)
	}
	if _, err := s.AddReview(ctx, taskID, AddReviewRequest{OperationID: "review-1", Generation: 1, PersonID: "rev-1", Decision: "pass"}); err != nil {
		t.Fatalf("review 1: %v", err)
	}
	if _, err := s.AddReview(ctx, taskID, AddReviewRequest{OperationID: "review-2", Generation: 1, PersonID: "rev-2", Decision: "pass"}); err != nil {
		t.Fatalf("review 2: %v", err)
	}
}

func modelCreateTask(ctx context.Context, t *testing.T, s *Service, operationID string) string {
	t.Helper()
	created, err := s.CreateTask(ctx, CreateTaskRequest{OperationID: operationID, Generation: 1})
	if err != nil {
		t.Fatalf("create %s: %v", operationID, err)
	}
	return created.TaskID
}

func modelLockRequest(suffix string) LockTaskRequest {
	req := lockRequest()
	req.OperationID = "lock-" + suffix
	req.BatchNo = "batch-" + suffix
	req.Seals = []SealSpec{{SealNo: "seal-" + suffix, Positions: []int{1, 2, 3}}}
	req.BlindCodes = []string{"blind-" + suffix + "-1", "blind-" + suffix + "-2"}
	return req
}
