package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
)

func modelCompletedTask(t *testing.T, srv *Server, verdict string) string {
	t.Helper()
	ctx := context.Background()
	svc := srv.svc

	created, err := svc.CreateTask(ctx, service.CreateTaskRequest{OperationID: "create", Generation: 1})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	id := created.TaskID
	_, err = svc.LockTask(ctx, id, service.LockTaskRequest{
		OperationID:       "lock",
		Generation:        1,
		HouseID:           "house-1",
		ShiftID:           "shift-1",
		FumigationBatchID: "fum-1",
		FumigationDigest:  "fum-digest-0001",
		RuleSetVersion:    1,
		BatchNo:           "batch-001",
		IncubatorSlotID:   "slot-1",
		CandlingWindowID:  "window-1",
		Seals:             []service.SealSpec{{SealNo: "seal-1", Positions: []int{1}}},
		BlindCodes:        []string{"blind-1"},
		CultureWells:      []string{"cw-1"},
		RapidWells:        []string{"rw-1"},
	})
	if err != nil {
		t.Fatalf("lock task: %v", err)
	}
	for i, person := range []string{"recv-1", "recv-2"} {
		if _, err := svc.AddReceipt(ctx, id, service.AddReceiptRequest{
			OperationID: "receipt-" + person, Generation: 1, PersonID: person,
		}); err != nil {
			t.Fatalf("receipt %d: %v", i+1, err)
		}
	}
	if _, err := svc.Start(ctx, id, service.StartRequest{OperationID: "start", Generation: 1}); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if _, err := svc.SubmitCandling(ctx, id, service.SubmitCandlingRequest{
		OperationID: "candling", Generation: 1,
		Entries: []service.CandlingEntryInput{{SealNo: "seal-1", Position: 1, Category: "fertile"}},
	}); err != nil {
		t.Fatalf("submit candling: %v", err)
	}
	if _, err := svc.SealSwab(ctx, id, service.SealSwabRequest{
		OperationID: "swab", Generation: 1, SealNo: "seal-1",
	}); err != nil {
		t.Fatalf("seal swab: %v", err)
	}
	if _, err := svc.SubmitCultureReading(ctx, id, service.CultureReadingRequest{
		OperationID: "culture", Generation: 1, Well: "cw-1", DeviceID: "dev-culture",
	}); err != nil {
		t.Fatalf("culture reading: %v", err)
	}
	if _, err := svc.SubmitRapidTest(ctx, id, service.RapidTestRequest{
		OperationID: "rapid", Generation: 1, Well: "rw-1", DeviceID: "dev-reader",
	}); err != nil {
		t.Fatalf("rapid-test reading: %v", err)
	}
	for _, kind := range []string{"egg_weight", "air_cell_height", "cleanliness", "fumigation_residue"} {
		if _, err := svc.SubmitPhysicochemical(ctx, id, service.PhysicochemicalRequest{
			OperationID: "phys-" + kind, Generation: 1, SealNo: "seal-1", Position: 1,
			Kind: kind, DeviceID: "dev-scale",
		}); err != nil {
			t.Fatalf("physicochemical %s: %v", kind, err)
		}
	}
	if _, err := svc.RevealBlind(ctx, id, service.RevealBlindRequest{
		OperationID: "reveal", Generation: 1, Codes: []string{"blind-1"},
	}); err != nil {
		t.Fatalf("reveal blind: %v", err)
	}
	if verdict != "" {
		if _, err := svc.CreateRetest(ctx, id, service.CreateRetestRequest{
			OperationID: "retest", Generation: 1, Trigger: "suspect_positive",
			AffectedSeals: []string{"seal-1"}, AffectedPositions: []int{1}, AffectedWells: []string{"cw-1"},
		}); err != nil {
			t.Fatalf("create retest: %v", err)
		}
		if _, err := svc.AddRetestEvidence(ctx, id, service.RetestEvidenceRequest{
			OperationID: "retest-evidence", Generation: 1, Kind: "colony_count", Value: 12, Verdict: verdict,
		}); err != nil {
			t.Fatalf("resolve %s retest: %v", verdict, err)
		}
	}
	for i, person := range []string{"rev-1", "rev-2"} {
		if _, err := svc.AddReview(ctx, id, service.AddReviewRequest{
			OperationID: "review-" + person, Generation: 1, PersonID: person, Decision: "pass",
		}); err != nil {
			t.Fatalf("review %d after %q retest: %v", i+1, verdict, err)
		}
	}
	return id
}

func modelRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestModel_RetestVerdictGatesTerminalDecision(t *testing.T) {
	tests := []struct {
		name           string
		verdict        string
		rejectedKind   string
		acceptedKind   string
		finalStatus    string
		wantCredential bool
	}{
		{name: "contaminated retest isolates", verdict: "contaminated", rejectedKind: "admit", acceptedKind: "isolate", finalStatus: "isolated"},
		{name: "clean retest admits", verdict: "clean", rejectedKind: "isolate", acceptedKind: "admit", finalStatus: "admitted", wantCredential: true},
		{name: "no retest admits", rejectedKind: "isolate", acceptedKind: "admit", finalStatus: "admitted", wantCredential: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			id := modelCompletedTask(t, srv, tt.verdict)
			decisionBody := service.FinalDecisionRequest{OperationID: "rejected-final", Generation: 1}

			rejected := modelRequest(t, srv, http.MethodPost, "/v1/tasks/"+id+"/decisions/"+tt.rejectedKind, decisionBody)
			if rejected.Code >= http.StatusOK && rejected.Code < http.StatusMultipleChoices {
				t.Fatalf("%s after verdict %q returned %d; decision must be rejected", tt.rejectedKind, tt.verdict, rejected.Code)
			}
			beforeFinal := modelRequest(t, srv, http.MethodGet, "/v1/tasks/"+id+"/credential", nil)
			if beforeFinal.Code == http.StatusOK {
				t.Fatalf("credential exists after rejected %s decision", tt.rejectedKind)
			}

			decisionBody.OperationID = "accepted-final"
			accepted := modelRequest(t, srv, http.MethodPost, "/v1/tasks/"+id+"/decisions/"+tt.acceptedKind, decisionBody)
			if accepted.Code != http.StatusOK {
				t.Fatalf("%s after verdict %q returned %d: %s", tt.acceptedKind, tt.verdict, accepted.Code, accepted.Body.String())
			}
			var result service.CommandResult
			if err := json.Unmarshal(accepted.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode final decision response: %v", err)
			}
			if result.Status != tt.finalStatus {
				t.Errorf("final status = %q, want %q", result.Status, tt.finalStatus)
			}

			credential := modelRequest(t, srv, http.MethodGet, "/v1/tasks/"+id+"/credential", nil)
			if tt.wantCredential && credential.Code != http.StatusOK {
				t.Errorf("credential status = %d, want 200: %s", credential.Code, credential.Body.String())
			}
			if !tt.wantCredential && credential.Code == http.StatusOK {
				t.Errorf("isolated contaminated task exposed a credential: %s", credential.Body.String())
			}

			decisionBody.OperationID = "late-final"
			late := modelRequest(t, srv, http.MethodPost, "/v1/tasks/"+id+"/decisions/"+tt.rejectedKind, decisionBody)
			if late.Code != http.StatusConflict {
				t.Errorf("late competing decision status = %d, want 409", late.Code)
			}
		})
	}
}
