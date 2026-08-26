package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
)

func TestModel_ActiveRetestBlocksTerminalDecisions(t *testing.T) {
	tests := []struct {
		kind       string
		wantStatus string
	}{
		{kind: "admit", wantStatus: "admitted"},
		{kind: "isolate", wantStatus: "isolated"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			srv := newTestServer(t)
			svc := srv.svc
			ctx := context.Background()
			must := func(step string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s: %v", step, err)
				}
			}

			created, err := svc.CreateTask(ctx, service.CreateTaskRequest{OperationID: "create", Generation: 1})
			must("create task", err)
			id := created.TaskID
			_, err = svc.LockTask(ctx, id, service.LockTaskRequest{
				OperationID:       "lock",
				Generation:        1,
				HouseID:           "house-1",
				ShiftID:           "shift-1",
				FumigationBatchID: "fum-1",
				FumigationDigest:  "fum-digest-0001",
				RuleSetVersion:    1,
				BatchNo:           "batch-" + tt.kind,
				IncubatorSlotID:   "slot-1",
				CandlingWindowID:  "window-1",
				Seals:             []service.SealSpec{{SealNo: "seal-1", Positions: []int{1}}},
				BlindCodes:        []string{"blind-1", "blind-2"},
				CultureWells:      []string{"cw-1"},
				RapidWells:        []string{"rw-1"},
			})
			must("lock task", err)
			_, err = svc.AddReceipt(ctx, id, service.AddReceiptRequest{OperationID: "receipt-1", Generation: 1, PersonID: "recv-1"})
			must("first receipt", err)
			_, err = svc.AddReceipt(ctx, id, service.AddReceiptRequest{OperationID: "receipt-2", Generation: 1, PersonID: "recv-2"})
			must("second receipt", err)
			_, err = svc.Start(ctx, id, service.StartRequest{OperationID: "start", Generation: 1})
			must("start", err)
			_, err = svc.SubmitCandling(ctx, id, service.SubmitCandlingRequest{
				OperationID: "candling", Generation: 1,
				Entries: []service.CandlingEntryInput{{SealNo: "seal-1", Position: 1, Category: "fertile"}},
			})
			must("candling", err)
			_, err = svc.SealSwab(ctx, id, service.SealSwabRequest{OperationID: "swab", Generation: 1, SealNo: "seal-1"})
			must("seal swab", err)
			_, err = svc.SubmitCultureReading(ctx, id, service.CultureReadingRequest{OperationID: "culture", Generation: 1, Well: "cw-1", DeviceID: "dev-culture"})
			must("culture reading", err)
			_, err = svc.SubmitRapidTest(ctx, id, service.RapidTestRequest{OperationID: "rapid", Generation: 1, Well: "rw-1", DeviceID: "dev-reader"})
			must("rapid reading", err)
			for _, kind := range []string{"egg_weight", "air_cell_height", "cleanliness", "fumigation_residue"} {
				_, err = svc.SubmitPhysicochemical(ctx, id, service.PhysicochemicalRequest{
					OperationID: "phys-" + kind, Generation: 1, SealNo: "seal-1", Position: 1,
					Kind: kind, DeviceID: "dev-scale",
				})
				must("physicochemical "+kind, err)
			}
			_, err = svc.CreateRetest(ctx, id, service.CreateRetestRequest{
				OperationID: "open-retest", Generation: 1, Trigger: "culture_pollution",
				AffectedSeals: []string{"seal-1"}, AffectedWells: []string{"cw-1"},
			})
			must("open retest", err)
			_, err = svc.RevealBlind(ctx, id, service.RevealBlindRequest{OperationID: "reveal", Generation: 1, Codes: []string{"blind-1", "blind-2"}})
			must("reveal blind", err)
			_, err = svc.AddReview(ctx, id, service.AddReviewRequest{OperationID: "review-1", Generation: 1, PersonID: "rev-1", Decision: "pass"})
			must("first review", err)
			_, err = svc.AddReview(ctx, id, service.AddReviewRequest{OperationID: "review-2", Generation: 1, PersonID: "rev-2", Decision: "pass"})
			must("second review", err)

			postDecision := func(operationID string) *httptest.ResponseRecorder {
				t.Helper()
				body := []byte(fmt.Sprintf(`{"operation_id":%q,"generation":1}`, operationID))
				req := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+id+"/decisions/"+tt.kind, bytes.NewReader(body))
				rec := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rec, req)
				return rec
			}

			blocked := postDecision("blocked-final")
			if blocked.Code != http.StatusConflict {
				t.Fatalf("active retest decision status = %d, want %d; body=%s", blocked.Code, http.StatusConflict, blocked.Body.String())
			}
			var apiErr ErrorResponse
			if err := json.Unmarshal(blocked.Body.Bytes(), &apiErr); err != nil {
				t.Fatalf("decode active retest response: %v", err)
			}
			if apiErr.Code != "evidence_incomplete" {
				t.Fatalf("active retest error code = %q, want evidence_incomplete", apiErr.Code)
			}
			view, err := svc.GetTask(ctx, id)
			must("get task after blocked decision", err)
			if view.Status != "admittable" || view.FinalKind != "" {
				t.Fatalf("blocked decision mutated task: status=%q final_kind=%q", view.Status, view.FinalKind)
			}
			if tt.kind == "admit" {
				if _, err := svc.GetCredential(ctx, id); err == nil {
					t.Fatal("blocked admit created an incubation credential")
				}
			}

			_, err = svc.AddRetestEvidence(ctx, id, service.RetestEvidenceRequest{
				OperationID: "resolve-retest", Generation: 1, Kind: "culture_colony", Value: 0, Verdict: "clean",
			})
			must("resolve retest", err)
			committed := postDecision("committed-final")
			if committed.Code != http.StatusOK {
				t.Fatalf("resolved retest decision status = %d, want %d; body=%s", committed.Code, http.StatusOK, committed.Body.String())
			}
			view, err = svc.GetTask(ctx, id)
			must("get task after committed decision", err)
			if view.Status != tt.wantStatus || view.FinalKind != tt.kind {
				t.Fatalf("committed decision task: status=%q final_kind=%q, want status=%q final_kind=%q", view.Status, view.FinalKind, tt.wantStatus, tt.kind)
			}
			if tt.kind == "admit" {
				cred, err := svc.GetCredential(ctx, id)
				must("get admitted credential", err)
				if cred.Number == "" {
					t.Fatal("successful admit did not create an incubation credential")
				}
			}
		})
	}
}
