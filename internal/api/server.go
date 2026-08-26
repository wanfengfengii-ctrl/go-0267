// Package api implements the Go HTTP API: command/query routing, input limits,
// stable error codes, deterministic reason ordering, transaction boundaries,
// request tracing and health checks.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
)

// Server wires the HTTP routes for the incubation inspection service.
type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

// NewServer constructs the HTTP handler around an application service.
func NewServer(svc *service.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", s.handleHealthz)

	m.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	m.HandleFunc("POST /v1/tasks/{id}/lock", s.handleLockTask)
	m.HandleFunc("POST /v1/tasks/{id}/receipts", s.handleReceipt)
	m.HandleFunc("POST /v1/tasks/{id}/start", s.handleStart)
	m.HandleFunc("POST /v1/tasks/{id}/window-exchanges", s.handleExchange)
	m.HandleFunc("POST /v1/tasks/{id}/candling", s.handleCandling)
	m.HandleFunc("POST /v1/tasks/{id}/swabs/seal", s.handleSealSwab)
	m.HandleFunc("POST /v1/tasks/{id}/cultures/readings", s.handleCulture)
	m.HandleFunc("POST /v1/tasks/{id}/rapid-tests/readings", s.handleRapidTest)
	m.HandleFunc("POST /v1/tasks/{id}/physicochemical", s.handlePhysicochemical)
	m.HandleFunc("POST /v1/tasks/{id}/blind/reveal", s.handleRevealBlind)
	m.HandleFunc("POST /v1/tasks/{id}/retests", s.handleCreateRetest)
	m.HandleFunc("POST /v1/tasks/{id}/retests/{generation}/evidence", s.handleRetestEvidence)
	m.HandleFunc("POST /v1/tasks/{id}/reviews", s.handleReview)
	m.HandleFunc("POST /v1/tasks/{id}/decisions/{kind}", s.handleDecision)

	m.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	m.HandleFunc("GET /v1/tasks/{id}/evidence", s.handleGetEvidence)
	m.HandleFunc("GET /v1/tasks/{id}/leases", s.handleGetLeases)
	m.HandleFunc("GET /v1/tasks/{id}/audit", s.handleGetAudit)
	m.HandleFunc("GET /v1/tasks/{id}/credential", s.handleGetCredential)
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
