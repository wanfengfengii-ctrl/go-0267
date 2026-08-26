package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/task"
)

// MeasureResult augments the uniform command result with device-retry state.
type MeasureResult struct {
	CommandResult
	PendingRetry bool  `json:"pending_retry"`
	Value        int64 `json:"value,omitempty"`
}

// ErrUnknownDevice is returned when a command names an unregistered device.
var ErrUnknownDevice = errors.New("service: unknown device")

// CandlingEntryInput is one position's candling submission.
type CandlingEntryInput struct {
	SealNo   string   `json:"seal_no"`
	Position int      `json:"position"`
	Category string   `json:"category"`
	Defects  []string `json:"defects,omitempty"`
	Retest   bool     `json:"retest"`
}

// SubmitCandlingRequest batches the candling coverage matrix.
type SubmitCandlingRequest struct {
	OperationID string               `json:"operation_id"`
	Generation  int                  `json:"generation"`
	Entries     []CandlingEntryInput `json:"entries"`
}

// SubmitCandling writes the coverage matrix; the whole batch rolls back unless
// the matrix exactly covers every locked position with valid classifications.
func (s *Service) SubmitCandling(ctx context.Context, taskID string, req SubmitCandlingRequest) (CommandResult, error) {
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != req.Generation {
			return ErrStaleGeneration
		}
		if t.Status == task.StatusResourcesOccupied {
			if err := advanceStatus(&t, task.StatusCandling); err != nil {
				return err
			}
		} else if t.Status != task.StatusCandling {
			return ErrInvalidState
		}

		entries := make([]evidence.CandlingEntry, 0, len(req.Entries))
		for _, in := range req.Entries {
			defects := make([]evidence.Defect, 0, len(in.Defects))
			for _, d := range in.Defects {
				defects = append(defects, evidence.Defect(d))
			}
			entries = append(entries, evidence.CandlingEntry{
				TaskID: taskID, SealNo: in.SealNo, Position: in.Position,
				Category: evidence.CandlingCategory(in.Category), Defects: defects,
				Retest: in.Retest, Version: 1,
			})
		}
		if err := evidence.ValidateEntries(entries); err != nil {
			return err
		}

		rr := store.NewResourceRepo(tx)
		seals, err := rr.Seals(ctx, taskID)
		if err != nil {
			return err
		}
		if err := validateCoverage(entries, seals); err != nil {
			return err
		}

		er := store.NewEvidenceRepo(tx)
		for _, e := range entries {
			if err := er.PutCandling(ctx, e); err != nil {
				return err
			}
		}

		if err := advanceStatus(&t, task.StatusSwabCulture); err != nil {
			return err
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "candling", "coverage complete", t.LogicalTime); err != nil {
			return err
		}
		out = newResult(t)
		return nil
	})
	return out, err
}

// validateCoverage checks the submitted matrix exactly covers each seal's
// locked positions (no duplicate or missing position).
func validateCoverage(entries []evidence.CandlingEntry, seals []resource.TraySeal) error {
	perSeal := map[string][]evidence.CandlingEntry{}
	for _, e := range entries {
		perSeal[e.SealNo] = append(perSeal[e.SealNo], e)
	}
	for _, seal := range seals {
		got := perSeal[seal.SealNo]
		if len(got) != len(seal.Positions) {
			if len(got) < len(seal.Positions) {
				return evidence.ErrMissingPosition
			}
			return evidence.ErrDuplicatePosition
		}
		seen := map[int]bool{}
		for _, e := range got {
			if seen[e.Position] {
				return evidence.ErrDuplicatePosition
			}
			seen[e.Position] = true
		}
		for _, p := range seal.Positions {
			if !seen[p.Position] {
				return evidence.ErrMissingPosition
			}
		}
	}
	return nil
}

// SealSwabRequest records the swab culture version for one seal.
type SealSwabRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	SealNo      string `json:"seal_no"`
}

// SealSwab seals a tray's eggshell swab.
func (s *Service) SealSwab(ctx context.Context, taskID string, req SealSwabRequest) (CommandResult, error) {
	var out CommandResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != req.Generation {
			return ErrStaleGeneration
		}
		if t.Status != task.StatusSwabCulture {
			return ErrInvalidState
		}
		er := store.NewEvidenceRepo(tx)
		if err := er.SealSwab(ctx, taskID, req.SealNo, t.Generation); err != nil {
			return err
		}
		t.LogicalTime++
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		if err := audit.Append(ctx, taskID, req.OperationID, "swab_sealed", req.SealNo, t.LogicalTime); err != nil {
			return err
		}
		out = newResult(t)
		return nil
	})
	return out, err
}

// CultureReadingRequest submits one culture-plate well reading via a device.
type CultureReadingRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	Well        string `json:"well"`
	DeviceID    string `json:"device_id"`
}

// SubmitCultureReading measures and records a colony count for a well.
func (s *Service) SubmitCultureReading(ctx context.Context, taskID string, req CultureReadingRequest) (MeasureResult, error) {
	return s.measureAndStore(ctx, taskID, req.OperationID, req.Generation, req.DeviceID,
		catalog.EvidenceColonyCount, req.Well, "culture", func(tx *sql.Tx, value int64, raw string) error {
			ar := store.NewArbitrationRepo(tx)
			return ar.PutCulture(ctx, arbitration.CultureEvidence{
				TaskID: taskID, Well: req.Well, Colony: value, Version: 1, Current: true, RawDigest: raw,
			})
		})
}

// RapidTestRequest submits one rapid-test well reading via a device.
type RapidTestRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	Well        string `json:"well"`
	DeviceID    string `json:"device_id"`
}

// SubmitRapidTest measures and records a Ct value for a well.
func (s *Service) SubmitRapidTest(ctx context.Context, taskID string, req RapidTestRequest) (MeasureResult, error) {
	return s.measureAndStore(ctx, taskID, req.OperationID, req.Generation, req.DeviceID,
		catalog.EvidenceCtValue, req.Well, "rapid", func(tx *sql.Tx, value int64, raw string) error {
			ar := store.NewArbitrationRepo(tx)
			return ar.PutRapidTest(ctx, arbitration.RapidTestEvidence{
				TaskID: taskID, Well: req.Well, CtValue: value, Version: 1, Current: true, RawDigest: raw,
			})
		})
}

// PhysicochemicalRequest submits one fixed-point physicochemical measurement.
type PhysicochemicalRequest struct {
	OperationID string `json:"operation_id"`
	Generation  int    `json:"generation"`
	SealNo      string `json:"seal_no"`
	Position    int    `json:"position"`
	Kind        string `json:"kind"`
	DeviceID    string `json:"device_id"`
}

// SubmitPhysicochemical measures and records one physicochemical evidence point.
func (s *Service) SubmitPhysicochemical(ctx context.Context, taskID string, req PhysicochemicalRequest) (MeasureResult, error) {
	return s.measureAndStore(ctx, taskID, req.OperationID, req.Generation, req.DeviceID,
		catalog.EvidenceKind(req.Kind), req.SealNo+"#"+itoa(req.Position), "physicochemical",
		func(tx *sql.Tx, value int64, raw string) error {
			er := store.NewEvidenceRepo(tx)
			return er.PutPhysicochemical(ctx, evidence.PhysicochemicalEvidence{
				TaskID: taskID, SealNo: req.SealNo, Position: req.Position,
				Kind: catalog.EvidenceKind(req.Kind), Raw: value, Version: 1,
			})
		})
}

// measureAndStore drives a device, parses the fixed-point result, and either
// stores evidence or records a pending retry attempt. It never fabricates a
// value on device failure, and advances the stage when the stage's evidence
// becomes complete.
func (s *Service) measureAndStore(ctx context.Context, taskID, opID string, generation int, deviceID string,
	kind catalog.EvidenceKind, object, stage string, storeFn func(*sql.Tx, int64, string) error) (MeasureResult, error) {

	var out MeasureResult
	err := s.store.InTx(ctx, func(tx *sql.Tx) error {
		tr := store.NewTaskRepo(tx)
		t, err := tr.Load(ctx, taskID)
		if err != nil {
			return ErrTaskNotFound
		}
		if t.Status.Terminal() {
			return ErrTerminal
		}
		if t.Generation != generation {
			return ErrStaleGeneration
		}
		if !stageAllowed(t.Status, stage) {
			return ErrInvalidState
		}

		cat := store.NewCatalogRepo(tx)
		rs, err := cat.RuleSet(ctx, t.RuleSnapshot)
		if err != nil {
			return err
		}
		precision, ok := rs.Precisions[kind]
		if !ok {
			return errors.New("service: no precision for kind")
		}
		dev, err := cat.Device(ctx, deviceID)
		if err != nil {
			return ErrUnknownDevice
		}
		if dev.Kind != expectedDeviceKind(kind) {
			return errors.New("service: device kind mismatch")
		}

		er := store.NewEvidenceRepo(tx)
		attemptNo := 1
		if prior, _ := er.Attempts(ctx, taskID); len(prior) > 0 {
			attemptNo = len(prior) + 1
		}
		attempt := evidence.DeviceAttempt{
			TaskID: taskID, DeviceID: deviceID, Kind: kind, Object: object,
			Generation: generation, Attempt: attemptNo, Pending: true,
		}
		port, ok := s.devices[deviceID]
		if !ok {
			return ErrUnknownDevice
		}
		raw, merr := port.Measure(ctx, attempt)
		if merr != nil || !validFixed(raw, precision) {
			if merr == nil {
				merr = evidence.ErrDeviceFormat
			}
			attempt.Failure = failureKind(merr)
			_, attempt.NextAt = evidence.NextRetry(t.LogicalTime, attemptNo)
			if err := er.PutAttempt(ctx, attempt); err != nil {
				return err
			}
			t.LogicalTime++
			if err := tr.Save(ctx, t); err != nil {
				return err
			}
			audit := store.NewAuditRepo(tx)
			_ = audit.Append(ctx, taskID, opID, "device_failed", string(attempt.Failure), t.LogicalTime)
			out = MeasureResult{CommandResult: newResult(t), PendingRetry: true}
			return nil
		}
		value, _ := evidence.ParseFixed(raw, precision)
		if err := storeFn(tx, value, raw); err != nil {
			return err
		}
		attempt.Pending = false
		if err := er.PutAttempt(ctx, attempt); err != nil {
			return err
		}
		t.LogicalTime++
		// Advance the stage when this stage's evidence is now complete.
		if err := s.maybeAdvance(ctx, tx, &t, stage); err != nil {
			return err
		}
		if err := tr.Save(ctx, t); err != nil {
			return err
		}
		audit := store.NewAuditRepo(tx)
		_ = audit.Append(ctx, taskID, opID, stage+"_recorded", object, t.LogicalTime)
		out = MeasureResult{CommandResult: newResult(t), Value: value}
		return nil
	})
	return out, err
}

func validFixed(raw string, precision int) bool {
	_, err := evidence.ParseFixed(raw, precision)
	return err == nil
}

func failureKind(err error) evidence.DeviceFailure {
	switch {
	case errors.Is(err, evidence.ErrDeviceRejected):
		return evidence.FailureRejected
	case errors.Is(err, evidence.ErrDeviceTimeout):
		return evidence.FailureTimeout
	case errors.Is(err, evidence.ErrDeviceFormat):
		return evidence.FailureFormat
	default:
		return evidence.FailureDown
	}
}

func stageAllowed(status task.TaskStatus, stage string) bool {
	switch stage {
	case "culture":
		return status == task.StatusSwabCulture
	case "rapid":
		return status == task.StatusRapidTest
	case "physicochemical":
		return status == task.StatusPhysicochemical
	default:
		return true
	}
}

// maybeAdvance moves the task to the next stage when the current stage's
// evidence has become complete.
func (s *Service) maybeAdvance(ctx context.Context, tx *sql.Tx, t *task.IncubationTask, stage string) error {
	rr := store.NewResourceRepo(tx)
	switch stage {
	case "culture":
		wells, err := rr.Active(ctx, t.ID)
		if err != nil {
			return err
		}
		var required []string
		for _, l := range wells {
			if l.Type == resource.LeaseCultureWell {
				required = append(required, l.ResourceKey)
			}
		}
		ar := store.NewArbitrationRepo(tx)
		read, err := ar.Culture(ctx, t.ID)
		if err != nil {
			return err
		}
		if covers(required, readWells(read)) {
			return advanceStatus(t, task.StatusRapidTest)
		}
	case "rapid":
		wells, err := rr.Active(ctx, t.ID)
		if err != nil {
			return err
		}
		var required []string
		for _, l := range wells {
			if l.Type == resource.LeaseRapidTestWell {
				required = append(required, l.ResourceKey)
			}
		}
		ar := store.NewArbitrationRepo(tx)
		read, err := ar.RapidTest(ctx, t.ID)
		if err != nil {
			return err
		}
		if covers(required, rapidWells(read)) {
			return advanceStatus(t, task.StatusPhysicochemical)
		}
	case "physicochemical":
		er := store.NewEvidenceRepo(tx)
		ev, err := er.Physicochemical(ctx, t.ID)
		if err != nil {
			return err
		}
		seen := map[catalog.EvidenceKind]bool{}
		for _, e := range ev {
			seen[e.Kind] = true
		}
		for _, k := range []catalog.EvidenceKind{
			catalog.EvidenceEggWeight, catalog.EvidenceAirCell,
			catalog.EvidenceCleanliness, catalog.EvidenceFumigation,
		} {
			if !seen[k] {
				return nil
			}
		}
		return advanceStatus(t, task.StatusPendingReview)
	}
	return nil
}

func readWells(rs []arbitration.CultureEvidence) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Well)
	}
	return out
}

func rapidWells(rs []arbitration.RapidTestEvidence) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Well)
	}
	return out
}

func covers(required, got []string) bool {
	if len(required) == 0 {
		return true
	}
	have := map[string]bool{}
	for _, g := range got {
		have[g] = true
	}
	for _, r := range required {
		if !have[r] {
			return false
		}
	}
	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// expectedDeviceKind maps an evidence kind to the device family that measures
// it, so a command cannot submit a reading from a mismatched device.
func expectedDeviceKind(kind catalog.EvidenceKind) catalog.DeviceKind {
	switch kind {
	case catalog.EvidenceColonyCount:
		return catalog.DeviceCultureBox
	case catalog.EvidenceCtValue:
		return catalog.DevicePlateReader
	default:
		return catalog.DeviceScale
	}
}
