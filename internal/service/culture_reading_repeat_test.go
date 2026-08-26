package service

import (
	"context"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

func TestModel_CultureRepeatReadingPersistsCurrentVersion(t *testing.T) {
	type cultureStats struct {
		rows        int
		currentRows int
		maxVersion  int
		currentSum  int64
	}

	newService := func(t *testing.T) *Service {
		t.Helper()
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return New(st)
	}

	createAndLock := func(t *testing.T, svc *Service) string {
		t.Helper()
		ctx := context.Background()
		created, err := svc.CreateTask(ctx, CreateTaskRequest{OperationID: "model-create", Generation: 1})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		locked, err := svc.LockTask(ctx, created.TaskID, LockTaskRequest{
			OperationID:       "model-lock",
			Generation:        1,
			HouseID:           "house-1",
			ShiftID:           "shift-1",
			FumigationBatchID: "fum-1",
			FumigationDigest:  "fum-digest-0001",
			RuleSetVersion:    1,
			BatchNo:           "model-batch",
			IncubatorSlotID:   "slot-1",
			CandlingWindowID:  "window-1",
			Seals:             []SealSpec{{SealNo: "seal-1", Positions: []int{1, 2, 3}}},
			BlindCodes:        []string{"blind-1", "blind-2"},
			CultureWells:      []string{"cw-1", "cw-2"},
			RapidWells:        []string{"rw-1"},
		})
		if err != nil {
			t.Fatalf("lock task: %v", err)
		}
		if locked.Status != string(task.StatusPendingReceipt) {
			t.Fatalf("locked status = %s, want pending_receipt", locked.Status)
		}
		return created.TaskID
	}

	advanceToCulture := func(t *testing.T, svc *Service, taskID string) {
		t.Helper()
		ctx := context.Background()
		for _, receipt := range []struct {
			op       string
			personID string
		}{
			{op: "model-receipt-1", personID: "recv-1"},
			{op: "model-receipt-2", personID: "recv-2"},
		} {
			if _, err := svc.AddReceipt(ctx, taskID, AddReceiptRequest{
				OperationID: receipt.op,
				Generation:  1,
				PersonID:    receipt.personID,
			}); err != nil {
				t.Fatalf("add receipt %s: %v", receipt.personID, err)
			}
		}
		if _, err := svc.Start(ctx, taskID, StartRequest{OperationID: "model-start", Generation: 1}); err != nil {
			t.Fatalf("start task: %v", err)
		}
		entries := []CandlingEntryInput{
			{SealNo: "seal-1", Position: 1, Category: "fertile"},
			{SealNo: "seal-1", Position: 2, Category: "fertile"},
			{SealNo: "seal-1", Position: 3, Category: "infertile"},
		}
		if _, err := svc.SubmitCandling(ctx, taskID, SubmitCandlingRequest{
			OperationID: "model-candling",
			Generation:  1,
			Entries:     entries,
		}); err != nil {
			t.Fatalf("submit candling: %v", err)
		}
		if _, err := svc.SealSwab(ctx, taskID, SealSwabRequest{
			OperationID: "model-swab",
			Generation:  1,
			SealNo:      "seal-1",
		}); err != nil {
			t.Fatalf("seal swab: %v", err)
		}
		view, err := svc.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if view.Status != string(task.StatusSwabCulture) {
			t.Fatalf("culture setup status = %s, want swab_culture", view.Status)
		}
	}

	readStats := func(t *testing.T, svc *Service, taskID, well string) cultureStats {
		t.Helper()
		var stats cultureStats
		err := svc.Store().DB().QueryRowContext(context.Background(), `SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN current=1 THEN 1 ELSE 0 END), 0),
			COALESCE(MAX(version), 0),
			COALESCE(SUM(CASE WHEN current=1 THEN colony ELSE 0 END), 0)
			FROM culture_evidence WHERE task_id=? AND well=?`, taskID, well).
			Scan(&stats.rows, &stats.currentRows, &stats.maxVersion, &stats.currentSum)
		if err != nil {
			t.Fatalf("read culture stats for %s: %v", well, err)
		}
		return stats
	}

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "repeat reading keeps latest current and does not advance until all wells complete",
			run: func(t *testing.T) {
				ctx := context.Background()
				svc := newService(t)
				readings := map[string][]string{
					"cw-1": {"5", "8"},
					"cw-2": {"13"},
				}
				svc.SetDevice("dev-culture", deviceFunc(func(ctx context.Context, attempt evidence.DeviceAttempt) (string, error) {
					queue := readings[attempt.Object]
					if len(queue) == 0 {
						t.Fatalf("unexpected culture read for %s", attempt.Object)
					}
					readings[attempt.Object] = queue[1:]
					return queue[0], nil
				}))
				taskID := createAndLock(t, svc)
				advanceToCulture(t, svc, taskID)

				first, err := svc.SubmitCultureReading(ctx, taskID, CultureReadingRequest{
					OperationID: "model-culture-cw1-v1",
					Generation:  1,
					Well:        "cw-1",
					DeviceID:    "dev-culture",
				})
				if err != nil {
					t.Fatalf("first culture reading: %v", err)
				}
				if first.PendingRetry || first.Value != 5 || first.Status != string(task.StatusSwabCulture) {
					t.Fatalf("first culture result = %+v, want value 5 without retry in swab_culture", first)
				}

				second, err := svc.SubmitCultureReading(ctx, taskID, CultureReadingRequest{
					OperationID: "model-culture-cw1-v2",
					Generation:  1,
					Well:        "cw-1",
					DeviceID:    "dev-culture",
				})
				if err != nil {
					t.Fatalf("repeat culture reading: %v", err)
				}
				if second.PendingRetry || second.Value != 8 || second.Status != string(task.StatusSwabCulture) {
					t.Fatalf("repeat culture result = %+v, want value 8 without retry still in swab_culture", second)
				}
				stats := readStats(t, svc, taskID, "cw-1")
				if stats.rows != 2 || stats.currentRows != 1 || stats.maxVersion != 2 || stats.currentSum != 8 {
					t.Fatalf("cw-1 stats = %+v, want two versions, one current v2 with value 8", stats)
				}
				ev, err := svc.GetEvidence(ctx, taskID)
				if err != nil {
					t.Fatalf("get evidence after repeat: %v", err)
				}
				if len(ev.Culture) != 1 || ev.Culture[0].Well != "cw-1" ||
					ev.Culture[0].Version != 2 || ev.Culture[0].Colony != 8 || !ev.Culture[0].Current {
					t.Fatalf("current culture view after repeat = %+v, want only cw-1 version 2 value 8 current", ev.Culture)
				}

				complete, err := svc.SubmitCultureReading(ctx, taskID, CultureReadingRequest{
					OperationID: "model-culture-cw2-v1",
					Generation:  1,
					Well:        "cw-2",
					DeviceID:    "dev-culture",
				})
				if err != nil {
					t.Fatalf("second well culture reading: %v", err)
				}
				if complete.Status != string(task.StatusRapidTest) {
					t.Fatalf("status after all culture wells = %s, want rapid_test", complete.Status)
				}
			},
		},
		{
			name: "device failure only records pending retry",
			run: func(t *testing.T) {
				ctx := context.Background()
				svc := newService(t)
				svc.SetDevice("dev-culture", deviceFunc(func(ctx context.Context, attempt evidence.DeviceAttempt) (string, error) {
					return "", evidence.ErrDeviceTimeout
				}))
				taskID := createAndLock(t, svc)
				advanceToCulture(t, svc, taskID)

				res, err := svc.SubmitCultureReading(ctx, taskID, CultureReadingRequest{
					OperationID: "model-culture-timeout",
					Generation:  1,
					Well:        "cw-1",
					DeviceID:    "dev-culture",
				})
				if err != nil {
					t.Fatalf("failed device culture reading: %v", err)
				}
				if !res.PendingRetry || res.Status != string(task.StatusSwabCulture) {
					t.Fatalf("failed device result = %+v, want pending retry in swab_culture", res)
				}
				pending, err := svc.PendingRetries(ctx, taskID)
				if err != nil {
					t.Fatalf("pending retries: %v", err)
				}
				if len(pending) != 1 || pending[0].Object != "cw-1" || pending[0].Failure != evidence.FailureTimeout {
					t.Fatalf("pending retries = %+v, want one timeout for cw-1", pending)
				}
				ev, err := svc.GetEvidence(ctx, taskID)
				if err != nil {
					t.Fatalf("get evidence after failure: %v", err)
				}
				if len(ev.Culture) != 0 {
					t.Fatalf("culture evidence after device failure = %+v, want none", ev.Culture)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
