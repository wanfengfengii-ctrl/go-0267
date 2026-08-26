package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/api"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
)

func TestModel_WindowExchangeTransaction(t *testing.T) {
	type commandResult struct {
		TaskID      string `json:"task_id"`
		Status      string `json:"status"`
		Generation  int    `json:"generation"`
		LogicalTime int64  `json:"logical_time"`
	}
	type taskView struct {
		Status      string `json:"status"`
		Generation  int    `json:"generation"`
		LogicalTime int64  `json:"logical_time"`
	}
	type leaseView struct {
		Type        string `json:"type"`
		ResourceKey string `json:"resource_key"`
		Generation  int    `json:"generation"`
		AcquiredAt  int64  `json:"acquired_at"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	type auditEntry struct {
		OperationID string `json:"operation_id"`
		Event       string `json:"event"`
		Detail      string `json:"detail"`
		LogicalTime int64  `json:"logical_time"`
	}
	type errorResponse struct {
		Code string `json:"code"`
	}

	cases := []struct {
		name        string
		leaseType   string
		fromKey     string
		toKey       string
		conflicting bool
	}{
		{name: "occupied candling window rolls back", leaseType: "candling_window", fromKey: "window-1", toKey: "window-2", conflicting: true},
		{name: "occupied incubator slot rolls back", leaseType: "incubator_slot", fromKey: "slot-1", toKey: "slot-2", conflicting: true},
		{name: "free candling window swaps atomically", leaseType: "candling_window", fromKey: "window-1", toKey: "window-2"},
		{name: "free incubator slot swaps atomically", leaseType: "incubator_slot", fromKey: "slot-1", toKey: "slot-2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			h := api.NewServer(service.New(st)).Handler()

			do := func(method, path string, body any) *httptest.ResponseRecorder {
				t.Helper()
				var data []byte
				if body != nil {
					data, err = json.Marshal(body)
					if err != nil {
						t.Fatalf("marshal %s: %v", path, err)
					}
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(data))
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec
			}
			decodeOK := func(rec *httptest.ResponseRecorder, dst any) {
				t.Helper()
				if rec.Code < 200 || rec.Code >= 300 {
					t.Fatalf("request returned status %d: %s", rec.Code, rec.Body.String())
				}
				if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
					t.Fatalf("decode response: %v", err)
				}
			}
			createLocked := func(suffix, slot, window string) commandResult {
				t.Helper()
				var created commandResult
				decodeOK(do(http.MethodPost, "/v1/tasks", map[string]any{
					"operation_id": "create-" + suffix, "generation": 1,
				}), &created)
				var locked commandResult
				decodeOK(do(http.MethodPost, fmt.Sprintf("/v1/tasks/%s/lock", created.TaskID), map[string]any{
					"operation_id": "lock-" + suffix, "generation": 1,
					"house_id": "house-1", "shift_id": "shift-1",
					"fumigation_batch_id": "fum-1", "fumigation_digest": "fum-digest-0001",
					"rule_set_version": 1, "batch_no": "batch-" + suffix,
					"incubator_slot_id": slot, "candling_window_id": window,
					"seals":         []map[string]any{{"seal_no": "seal-" + suffix, "positions": []int{1}}},
					"blind_codes":   []string{"blind-" + suffix},
					"culture_wells": []string{"culture-" + suffix},
					"rapid_wells":   []string{"rapid-" + suffix},
				}), &locked)
				return locked
			}
			getTask := func(id string) taskView {
				t.Helper()
				var v taskView
				decodeOK(do(http.MethodGet, "/v1/tasks/"+id, nil), &v)
				return v
			}
			getLeases := func(id string) []leaseView {
				t.Helper()
				var v []leaseView
				decodeOK(do(http.MethodGet, "/v1/tasks/"+id+"/leases", nil), &v)
				return v
			}
			getAudit := func(id string) []auditEntry {
				t.Helper()
				var v []auditEntry
				decodeOK(do(http.MethodGet, "/v1/tasks/"+id+"/audit", nil), &v)
				return v
			}

			first := createLocked("one", "slot-1", "window-1")
			if tc.conflicting {
				createLocked("two", "slot-2", "window-2")
			}
			beforeTask := getTask(first.TaskID)
			beforeLeases := getLeases(first.TaskID)
			beforeAudit := getAudit(first.TaskID)

			rec := do(http.MethodPost, "/v1/tasks/"+first.TaskID+"/window-exchanges", map[string]any{
				"operation_id": "exchange", "generation": 1, "type": tc.leaseType,
				"from_key": tc.fromKey, "to_key": tc.toKey,
			})

			if tc.conflicting {
				var gotErr errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &gotErr); err != nil {
					t.Fatalf("decode conflict: %v", err)
				}
				if rec.Code != http.StatusConflict || gotErr.Code != "resource_conflict" {
					t.Errorf("exchange status/code = %d/%q, want 409/resource_conflict; body=%s", rec.Code, gotErr.Code, rec.Body.String())
				}
				if got := getTask(first.TaskID); !reflect.DeepEqual(got, beforeTask) {
					t.Errorf("failed exchange changed task: before=%+v after=%+v", beforeTask, got)
				}
				if got := getLeases(first.TaskID); !reflect.DeepEqual(got, beforeLeases) {
					t.Errorf("failed exchange changed leases: before=%+v after=%+v", beforeLeases, got)
				}
				if got := getAudit(first.TaskID); !reflect.DeepEqual(got, beforeAudit) {
					t.Errorf("failed exchange appended audit: before=%+v after=%+v", beforeAudit, got)
				}
				return
			}

			var exchanged commandResult
			decodeOK(rec, &exchanged)
			afterTask := getTask(first.TaskID)
			if afterTask.Status != beforeTask.Status || afterTask.Generation != beforeTask.Generation || afterTask.LogicalTime != beforeTask.LogicalTime+1 {
				t.Errorf("successful exchange task = %+v, before=%+v", afterTask, beforeTask)
			}
			afterLeases := getLeases(first.TaskID)
			if len(afterLeases) != len(beforeLeases) {
				t.Fatalf("lease count after exchange = %d, want %d", len(afterLeases), len(beforeLeases))
			}
			var oldHeld, newHeld bool
			for _, lease := range afterLeases {
				if lease.Type == tc.leaseType && lease.ResourceKey == tc.fromKey {
					oldHeld = true
				}
				if lease.Type == tc.leaseType && lease.ResourceKey == tc.toKey {
					newHeld = true
				}
			}
			if oldHeld || !newHeld {
				t.Errorf("successful exchange leases = %+v, want %s released and %s acquired", afterLeases, tc.fromKey, tc.toKey)
			}
			afterAudit := getAudit(first.TaskID)
			if len(afterAudit) != len(beforeAudit)+1 || afterAudit[len(afterAudit)-1].Event != "window_exchange" {
				t.Errorf("successful exchange audit = %+v, want one window_exchange entry", afterAudit)
			}
		})
	}
}
