package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

func TestModel_DeviceRetryLifecycle(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "failed reading is only a pending audited attempt",
			run: func(t *testing.T) {
				s := newTestService(t)
				s.SetDevice("dev-culture", failingDevice(evidence.ErrDeviceTimeout))
				ctx := context.Background()
				id, _ := createAndLock(t, s)
				advanceToCulture(t, s, id)

				result, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{
					OperationID: "culture-timeout", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
				})
				if err != nil {
					t.Fatalf("submit failed reading: %v", err)
				}
				if !result.PendingRetry || result.Value != 0 {
					t.Fatalf("failed reading result = %+v, want pending retry without a value", result)
				}

				view, err := s.GetEvidence(ctx, id)
				if err != nil {
					t.Fatalf("get evidence: %v", err)
				}
				if len(view.Culture) != 0 {
					t.Fatalf("culture evidence count = %d, want 0", len(view.Culture))
				}
				if len(view.DeviceAttempts) != 1 || !view.DeviceAttempts[0].Pending || view.DeviceAttempts[0].Failure != evidence.FailureTimeout {
					t.Fatalf("device attempts = %+v, want one pending timeout", view.DeviceAttempts)
				}
				taskView, err := s.GetTask(ctx, id)
				if err != nil {
					t.Fatalf("get task: %v", err)
				}
				if taskView.Status != string(task.StatusSwabCulture) {
					t.Fatalf("status after failed reading = %s, want %s", taskView.Status, task.StatusSwabCulture)
				}
			},
		},
		{
			name: "successful reading resolves the same retry key across restart",
			run: func(t *testing.T) {
				ctx := context.Background()
				dbPath := filepath.Join(t.TempDir(), "retry.db")
				st, err := store.Open(dbPath)
				if err != nil {
					t.Fatalf("open store: %v", err)
				}
				defer func() {
					if st != nil {
						_ = st.Close()
					}
				}()
				s := New(st)
				s.SetDevice("dev-culture", failingDevice(evidence.ErrDeviceTimeout))
				id, _ := createAndLock(t, s)
				advanceToCulture(t, s, id)

				failed, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{
					OperationID: "culture-timeout", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
				})
				if err != nil {
					t.Fatalf("submit failed reading: %v", err)
				}
				if !failed.PendingRetry {
					t.Fatal("failed reading did not request a retry")
				}

				s.SetDevice("dev-culture", deviceFunc(func(context.Context, evidence.DeviceAttempt) (string, error) {
					return "5", nil
				}))
				succeeded, err := s.SubmitCultureReading(ctx, id, CultureReadingRequest{
					OperationID: "culture-retry", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
				})
				if err != nil {
					t.Fatalf("submit successful retry: %v", err)
				}
				if succeeded.PendingRetry || succeeded.Status != string(task.StatusRapidTest) {
					t.Fatalf("successful retry result = %+v, want completed reading in rapid-test stage", succeeded)
				}

				view, err := s.GetEvidence(ctx, id)
				if err != nil {
					t.Fatalf("get evidence: %v", err)
				}
				if len(view.Culture) != 1 || view.Culture[0].Colony != 5 {
					t.Fatalf("culture evidence = %+v, want one valid reading of 5", view.Culture)
				}
				if len(view.DeviceAttempts) != 2 {
					t.Fatalf("device attempt count = %d, want 2", len(view.DeviceAttempts))
				}
				first, second := view.DeviceAttempts[0], view.DeviceAttempts[1]
				if first.TaskID != second.TaskID || first.DeviceID != second.DeviceID || first.Kind != second.Kind || first.Object != second.Object || first.Generation != second.Generation {
					t.Fatalf("attempt retry keys differ: first=%+v second=%+v", first, second)
				}
				if first.Pending || second.Pending {
					t.Fatalf("resolved evidence still exposes pending attempts: %+v", view.DeviceAttempts)
				}

				if err := st.Close(); err != nil {
					t.Fatalf("close store: %v", err)
				}
				st = nil
				reopened, err := store.Open(dbPath)
				if err != nil {
					t.Fatalf("reopen store: %v", err)
				}
				defer reopened.Close()
				pending, err := New(reopened).PendingRetries(ctx, id)
				if err != nil {
					t.Fatalf("recover pending retries: %v", err)
				}
				if len(pending) != 0 {
					t.Fatalf("pending retries after restart = %+v, want none", pending)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
