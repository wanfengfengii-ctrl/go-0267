package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/service"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(service.New(st))
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestHealthzNotFoundMethod(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("POST /healthz should not return 200")
	}
}

func TestSortReasonsDeterministic(t *testing.T) {
	in := []Reason{
		{Code: "E1", TraySeal: "B", BatchNo: "Z", HouseID: "H2", Position: 3, Well: "W2"},
		{Code: "E0", TraySeal: "A", BatchNo: "Z", HouseID: "H2", Position: 1, Well: "W1"},
		{Code: "E9", TraySeal: "A", BatchNo: "Y", HouseID: "H1", Position: 1, Well: "W1"},
	}
	// Shuffle input across many orders; result must always be identical.
	want := []Reason{
		{Code: "E9", TraySeal: "A", BatchNo: "Y", HouseID: "H1", Position: 1, Well: "W1"},
		{Code: "E0", TraySeal: "A", BatchNo: "Z", HouseID: "H2", Position: 1, Well: "W1"},
		{Code: "E1", TraySeal: "B", BatchNo: "Z", HouseID: "H2", Position: 3, Well: "W2"},
	}
	permutations := [][]Reason{
		in,
		{in[2], in[0], in[1]},
		{in[1], in[2], in[0]},
	}
	for _, p := range permutations {
		cp := append([]Reason(nil), p...)
		SortReasons(cp)
		if !reflect.DeepEqual(cp, want) {
			t.Errorf("SortReasons(%v) = %v, want %v", p, cp, want)
		}
	}
}

func TestSortReasonsTieBreakByCode(t *testing.T) {
	in := []Reason{{Code: "E2"}, {Code: "E1"}}
	SortReasons(in)
	if in[0].Code != "E1" || in[1].Code != "E2" {
		t.Errorf("codes not ordered: %v", in)
	}
}
