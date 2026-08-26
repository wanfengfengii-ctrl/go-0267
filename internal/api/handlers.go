package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/arbitration"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/resource"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
)

// maxBody limits request size to keep input handling bounded.
const maxBody = 1 << 20

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// mapError converts a service/domain error into a stable HTTP error.
func mapError(err error) (int, string, string, []Reason) {
	reasons := []Reason{}
	switch {
	case errors.Is(err, service.ErrTaskNotFound):
		return http.StatusNotFound, "task_not_found", "task does not exist", reasons
	case errors.Is(err, service.ErrTerminal):
		return http.StatusConflict, "task_terminal", "task is already terminal", reasons
	case errors.Is(err, service.ErrStaleGeneration):
		return http.StatusConflict, "stale_generation", "task generation is stale", reasons
	case errors.Is(err, service.ErrInvalidState):
		return http.StatusConflict, "invalid_state", "task is not in a valid state for this command", reasons
	case errors.Is(err, service.ErrOperationConflict):
		return http.StatusConflict, "operation_conflict", "operation id reused with different content", reasons
	case errors.Is(err, service.ErrNotQualified):
		return http.StatusForbidden, "not_qualified", "person is not qualified for this role", reasons
	case errors.Is(err, arbitration.ErrEvidenceIncomplete):
		return http.StatusConflict, "evidence_incomplete", "evidence is not closed", reasons
	case errors.Is(err, resource.ErrLeaseConflict):
		return http.StatusConflict, "resource_conflict", "resource already held by an open task", reasons
	case errors.Is(err, catalog.ErrStaleFumigation):
		return http.StatusUnprocessableEntity, "stale_fumigation", "fumigation digest is stale", reasons
	case errors.Is(err, catalog.ErrShiftHouseMismatch):
		return http.StatusUnprocessableEntity, "source_mismatch", "shift does not belong to house", reasons
	case errors.Is(err, catalog.ErrHouseNotFound), errors.Is(err, catalog.ErrShiftNotFound):
		return http.StatusUnprocessableEntity, "catalog_mismatch", "house or shift not found or not effective", reasons
	case errors.Is(err, catalog.ErrQualification), errors.Is(err, catalog.ErrPersonNotQualified):
		return http.StatusForbidden, "not_qualified", "person qualification missing", reasons
	case errors.Is(err, evidence.ErrDuplicatePosition), errors.Is(err, evidence.ErrMissingPosition):
		return http.StatusUnprocessableEntity, "matrix_invalid", "candling matrix does not cover positions exactly", reasons
	case errors.Is(err, evidence.ErrInvalidCategory), errors.Is(err, evidence.ErrInvalidDefect),
		errors.Is(err, evidence.ErrDefectMismatch):
		return http.StatusUnprocessableEntity, "matrix_invalid", "invalid candling classification or defect", reasons
	case errors.Is(err, evidence.ErrInvalidPrecision), errors.Is(err, evidence.ErrPrecision),
		errors.Is(err, evidence.ErrInvalidFormat), errors.Is(err, evidence.ErrInvalidLength),
		errors.Is(err, evidence.ErrOverflow):
		return http.StatusUnprocessableEntity, "fixedpoint_invalid", "invalid fixed-point value", reasons
	case errors.Is(err, arbitration.ErrReviewerIsReceiver):
		return http.StatusUnprocessableEntity, "reviewer_overlap", "reviewer overlaps receiver", reasons
	case errors.Is(err, arbitration.ErrDuplicateReviewer):
		return http.StatusUnprocessableEntity, "reviewer_duplicate", "reviewer already signed", reasons
	case errors.Is(err, service.ErrBlindPremature):
		return http.StatusConflict, "blind_premature", "blind reveal not allowed at this stage", reasons
	default:
		return http.StatusInternalServerError, "internal_error", "unexpected error", reasons
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg, reasons := mapError(err)
	opID := r.Header.Get("X-Operation-ID")
	SortReasons(reasons)
	writeJSON(w, status, ErrorResponse{Code: code, Message: msg, Reasons: reasons, OperationID: opID})
}

// --- Command handlers -------------------------------------------------------

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req service.CreateTaskRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	if req.OperationID == "" {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Code: "invalid_input", Message: "operation_id is required"})
		return
	}
	res, err := s.svc.CreateTask(r.Context(), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) handleLockTask(w http.ResponseWriter, r *http.Request) {
	var req service.LockTaskRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.LockTask(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleReceipt(w http.ResponseWriter, r *http.Request) {
	var req service.AddReceiptRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.AddReceipt(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req service.StartRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.Start(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExchange(w http.ResponseWriter, r *http.Request) {
	var req service.ExchangeRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.ExchangeWindow(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCandling(w http.ResponseWriter, r *http.Request) {
	var req service.SubmitCandlingRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.SubmitCandling(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSealSwab(w http.ResponseWriter, r *http.Request) {
	var req service.SealSwabRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.SealSwab(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCulture(w http.ResponseWriter, r *http.Request) {
	var req service.CultureReadingRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.SubmitCultureReading(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRapidTest(w http.ResponseWriter, r *http.Request) {
	var req service.RapidTestRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.SubmitRapidTest(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePhysicochemical(w http.ResponseWriter, r *http.Request) {
	var req service.PhysicochemicalRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.SubmitPhysicochemical(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRevealBlind(w http.ResponseWriter, r *http.Request) {
	var req service.RevealBlindRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.RevealBlind(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCreateRetest(w http.ResponseWriter, r *http.Request) {
	var req service.CreateRetestRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.CreateRetest(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRetestEvidence(w http.ResponseWriter, r *http.Request) {
	var req service.RetestEvidenceRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	gen, err := strconv.Atoi(r.PathValue("generation"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Code: "invalid_input", Message: "generation must be an integer"})
		return
	}
	req.Generation = gen
	res, err := s.svc.AddRetestEvidence(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req service.AddReviewRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.svc.AddReview(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	var req service.FinalDecisionRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	req.Kind = r.PathValue("kind")
	res, err := s.svc.FinalDecision(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// --- Query handlers ---------------------------------------------------------

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.GetEvidence(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetLeases(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.GetLeases(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.GetAudit(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetCredential(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.GetCredential(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
