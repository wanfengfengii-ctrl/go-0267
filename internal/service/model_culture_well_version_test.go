package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

func TestModel_CultureWellVersionChainsCloseStage(t *testing.T) {
	tests := []struct {
		name     string
		readings []struct {
			well  string
			value string
		}
		wantColonies map[string]int64
		wantVersions map[string]int
	}{
		{
			name: "different wells remain current and close culture stage",
			readings: []struct {
				well  string
				value string
			}{{"cw-a", "5"}, {"cw-b", "7"}},
			wantColonies: map[string]int64{"cw-a": 5, "cw-b": 7},
			wantVersions: map[string]int{"cw-a": 1, "cw-b": 1},
		},
		{
			name: "rereading one well replaces only its current version",
			readings: []struct {
				well  string
				value string
			}{{"cw-a", "3"}, {"cw-a", "9"}, {"cw-b", "4"}},
			wantColonies: map[string]int64{"cw-a": 9, "cw-b": 4},
			wantVersions: map[string]int{"cw-a": 2, "cw-b": 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t)
			ctx := context.Background()
			created, err := s.CreateTask(ctx, CreateTaskRequest{OperationID: "create", Generation: 1})
			if err != nil {
				t.Fatalf("create task: %v", err)
			}
			lock := lockRequest()
			lock.CultureWells = []string{"cw-a", "cw-b"}
			if _, err := s.LockTask(ctx, created.TaskID, lock); err != nil {
				t.Fatalf("lock task: %v", err)
			}
			advanceToCulture(t, s, created.TaskID)
			if _, err := s.SealSwab(ctx, created.TaskID, SealSwabRequest{
				OperationID: "swab", Generation: 1, SealNo: "seal-1",
			}); err != nil {
				t.Fatalf("seal swab: %v", err)
			}

			next := 0
			s.SetDevice("dev-culture", deviceFunc(func(context.Context, evidence.DeviceAttempt) (string, error) {
				if next >= len(tc.readings) {
					return "", fmt.Errorf("unexpected culture device call %d", next+1)
				}
				value := tc.readings[next].value
				next++
				return value, nil
			}))

			for i, reading := range tc.readings {
				_, err := s.SubmitCultureReading(ctx, created.TaskID, CultureReadingRequest{
					OperationID: fmt.Sprintf("culture-%d", i+1), Generation: 1,
					Well: reading.well, DeviceID: "dev-culture",
				})
				if err != nil {
					t.Fatalf("submit reading %d for %s: %v", i+1, reading.well, err)
				}
			}

			view, err := s.GetTask(ctx, created.TaskID)
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			if view.Status != string(task.StatusRapidTest) {
				t.Fatalf("status = %q, want %q", view.Status, task.StatusRapidTest)
			}
			ev, err := s.GetEvidence(ctx, created.TaskID)
			if err != nil {
				t.Fatalf("get evidence: %v", err)
			}
			if len(ev.Culture) != len(tc.wantColonies) {
				t.Fatalf("current culture count = %d, want %d: %+v", len(ev.Culture), len(tc.wantColonies), ev.Culture)
			}
			seen := make(map[string]bool, len(ev.Culture))
			for _, got := range ev.Culture {
				wantColony, ok := tc.wantColonies[got.Well]
				if !ok {
					t.Fatalf("unexpected current culture well %q", got.Well)
				}
				seen[got.Well] = true
				if !got.Current || got.Colony != wantColony || got.Version != tc.wantVersions[got.Well] {
					t.Errorf("current culture %s = {colony:%d version:%d current:%t}, want {colony:%d version:%d current:true}",
						got.Well, got.Colony, got.Version, got.Current, wantColony, tc.wantVersions[got.Well])
				}
			}
			for well := range tc.wantColonies {
				if !seen[well] {
					t.Errorf("current culture evidence is missing locked well %q", well)
				}
			}
		})
	}
}
