package service

import (
	"context"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
)

func TestModel_BlindRevealBatchAtomicity(t *testing.T) {
	tests := []struct {
		name       string
		preReveal  []string
		codes      []string
		wantErr    bool
		wantReveal map[string]bool
	}{
		{
			name:       "unknown code rolls back matching code",
			codes:      []string{"blind-1", "blind-x"},
			wantErr:    true,
			wantReveal: map[string]bool{"blind-1": false, "blind-2": false},
		},
		{
			name:       "duplicate code in request rolls back batch",
			codes:      []string{"blind-1", "blind-1"},
			wantErr:    true,
			wantReveal: map[string]bool{"blind-1": false, "blind-2": false},
		},
		{
			name:       "previously revealed code rolls back unrevealed code",
			preReveal:  []string{"blind-1"},
			codes:      []string{"blind-1", "blind-2"},
			wantErr:    true,
			wantReveal: map[string]bool{"blind-1": true, "blind-2": false},
		},
		{
			name:       "all unrevealed codes commit together",
			codes:      []string{"blind-1", "blind-2"},
			wantReveal: map[string]bool{"blind-1": true, "blind-2": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestService(t)
			id, _ := createAndLock(t, s)

			must := func(stage string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s: %v", stage, err)
				}
			}
			_, err := s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: "r1", Generation: 1, PersonID: "recv-1"})
			must("receipt 1", err)
			_, err = s.AddReceipt(ctx, id, AddReceiptRequest{OperationID: "r2", Generation: 1, PersonID: "recv-2"})
			must("receipt 2", err)
			_, err = s.Start(ctx, id, StartRequest{OperationID: "start", Generation: 1})
			must("start", err)
			_, err = s.SubmitCandling(ctx, id, SubmitCandlingRequest{
				OperationID: "candling",
				Generation:  1,
				Entries: []CandlingEntryInput{
					{SealNo: "seal-1", Position: 1, Category: "fertile"},
					{SealNo: "seal-1", Position: 2, Category: "fertile"},
					{SealNo: "seal-1", Position: 3, Category: "infertile"},
				},
			})
			must("candling", err)
			_, err = s.SealSwab(ctx, id, SealSwabRequest{OperationID: "swab", Generation: 1, SealNo: "seal-1"})
			must("seal swab", err)
			_, err = s.SubmitCultureReading(ctx, id, CultureReadingRequest{OperationID: "culture", Generation: 1, Well: "cw-1", DeviceID: "dev-culture"})
			must("culture", err)
			_, err = s.SubmitRapidTest(ctx, id, RapidTestRequest{OperationID: "rapid", Generation: 1, Well: "rw-1", DeviceID: "dev-reader"})
			must("rapid", err)
			for _, kind := range []string{"egg_weight", "air_cell_height", "cleanliness", "fumigation_residue"} {
				_, err = s.SubmitPhysicochemical(ctx, id, PhysicochemicalRequest{
					OperationID: "phys-" + kind,
					Generation:  1,
					SealNo:      "seal-1",
					Position:    1,
					Kind:        kind,
					DeviceID:    "dev-scale",
				})
				must("physicochemical "+kind, err)
			}

			if len(tt.preReveal) != 0 {
				_, err = s.RevealBlind(ctx, id, RevealBlindRequest{OperationID: "pre-reveal", Generation: 1, Codes: tt.preReveal})
				must("pre-reveal", err)
			}
			beforeTask, err := s.GetTask(ctx, id)
			must("get task before reveal", err)
			beforeAudit, err := s.GetAudit(ctx, id)
			must("get audit before reveal", err)

			_, err = s.RevealBlind(ctx, id, RevealBlindRequest{OperationID: "reveal-under-test", Generation: 1, Codes: tt.codes})
			if tt.wantErr && err == nil {
				t.Fatal("RevealBlind succeeded; want the entire invalid batch rejected")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("RevealBlind returned %v; want success", err)
			}

			afterTask, err := s.GetTask(ctx, id)
			must("get task after reveal", err)
			afterAudit, err := s.GetAudit(ctx, id)
			must("get audit after reveal", err)
			wantLogicalTime := beforeTask.LogicalTime
			wantAuditLen := len(beforeAudit)
			if !tt.wantErr {
				wantLogicalTime++
				wantAuditLen++
			}
			if afterTask.LogicalTime != wantLogicalTime {
				t.Errorf("logical time = %d, want %d", afterTask.LogicalTime, wantLogicalTime)
			}
			if len(afterAudit) != wantAuditLen {
				t.Errorf("audit entries = %d, want %d", len(afterAudit), wantAuditLen)
			}
			for _, entry := range afterAudit[len(beforeAudit):] {
				if entry.Event != "blind_revealed" {
					t.Errorf("new audit event = %q, want blind_revealed", entry.Event)
				}
			}

			blinds, err := store.NewResourceRepo(s.Store().DB()).Blinds(ctx, id)
			must("query blind samples", err)
			if len(blinds) != len(tt.wantReveal) {
				t.Fatalf("blind sample count = %d, want %d", len(blinds), len(tt.wantReveal))
			}
			for _, blind := range blinds {
				if blind.Revealed != tt.wantReveal[blind.Code] {
					t.Errorf("blind %q revealed = %v, want %v", blind.Code, blind.Revealed, tt.wantReveal[blind.Code])
				}
			}
		})
	}
}
