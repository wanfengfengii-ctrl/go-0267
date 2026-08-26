package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/api"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
)

func TestModel_CreateTaskOperationIdempotency(t *testing.T) {
	tests := []struct {
		name             string
		firstBody        string
		replayBody       string
		wantReplayStatus int
		wantErrorCode    string
	}{
		{
			name:             "same normalized request returns the original response",
			firstBody:        `{"operation_id":"create-timeout","generation":1}`,
			replayBody:       "{\n  \"generation\": 1,\n  \"operation_id\": \"create-timeout\"\n}",
			wantReplayStatus: http.StatusCreated,
		},
		{
			name:             "same operation id with different content conflicts",
			firstBody:        `{"operation_id":"create-conflict","generation":1}`,
			replayBody:       `{"operation_id":"create-conflict","generation":2}`,
			wantReplayStatus: http.StatusConflict,
			wantErrorCode:    "operation_conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			svc := service.New(st)
			handler := api.NewServer(svc).Handler()

			post := func(body string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewBufferString(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				return rec
			}

			first := post(tt.firstBody)
			if first.Code != http.StatusCreated {
				t.Fatalf("first POST status = %d, want %d; body=%s", first.Code, http.StatusCreated, first.Body.String())
			}
			var created service.CommandResult
			if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
				t.Fatalf("decode first response: %v", err)
			}
			if created.TaskID == "" {
				t.Fatal("first POST returned an empty task_id")
			}

			replay := post(tt.replayBody)
			if replay.Code != tt.wantReplayStatus {
				t.Fatalf("replayed POST status = %d, want %d; body=%s", replay.Code, tt.wantReplayStatus, replay.Body.String())
			}
			if tt.wantErrorCode == "" {
				if !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) {
					t.Fatalf("replayed response = %s, want original response %s", replay.Body.String(), first.Body.String())
				}
			} else {
				var got api.ErrorResponse
				if err := json.Unmarshal(replay.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode conflict response: %v", err)
				}
				if got.Code != tt.wantErrorCode {
					t.Errorf("replayed error code = %q, want %q", got.Code, tt.wantErrorCode)
				}
			}

			open, err := svc.OpenTasks(context.Background())
			if err != nil {
				t.Fatalf("list open tasks: %v", err)
			}
			if len(open) != 1 || open[0].ID != created.TaskID {
				t.Errorf("open tasks after replay = %+v, want only task %q", open, created.TaskID)
			}
			audit, err := svc.GetAudit(context.Background(), created.TaskID)
			if err != nil {
				t.Fatalf("get audit: %v", err)
			}
			if len(audit) != 1 || audit[0].Event != "created" {
				t.Errorf("audit after replay = %+v, want one created entry", audit)
			}
		})
	}
}
