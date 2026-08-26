// Package arbitration models the 污染复判及终局仲裁器 (pollution retest and
// terminal arbitrator): append-only culture/rapid/retest version chains,
// independent review, unique credentials, and the single-write terminal race
// between admit, isolate and cancel.
package arbitration

import (
	"context"
)

// CultureEvidence is an append-only culture-plate reading with a current flag.
type CultureEvidence struct {
	TaskID    string `json:"task_id"`
	Well      string `json:"well"`
	Colony    int64  `json:"colony"`
	Version   int    `json:"version"`
	Current   bool   `json:"current"`
	RawDigest string `json:"raw_digest"`
}

// RapidTestEvidence is an append-only rapid-test plate reading.
type RapidTestEvidence struct {
	TaskID    string `json:"task_id"`
	Well      string `json:"well"`
	CtValue   int64  `json:"ct_value"`
	Version   int    `json:"version"`
	Current   bool   `json:"current"`
	RawDigest string `json:"raw_digest"`
}

// RetestCase is the single active retest case for a task generation, listing
// every affected seal, position, blind code and detection well.
type RetestCase struct {
	TaskID            string
	Generation        int
	AffectedSeals     []string
	AffectedPositions []int
	AffectedBlinds    []string
	AffectedWells     []string
	Verdict           string
	Active            bool
}

// ReviewDecision is the conclusion a qualified reviewer signs.
type ReviewDecision string

const (
	ReviewPass ReviewDecision = "pass"
	ReviewFail ReviewDecision = "fail"
)

// IndependentReview is one reviewer's signed conclusion for a task generation.
type IndependentReview struct {
	TaskID     string
	PersonID   string
	Generation int
	Decision   ReviewDecision
}

// FinalDecisionKind enumerates the three competing terminal commands.
type FinalDecisionKind string

const (
	DecisionAdmit   FinalDecisionKind = "admit"
	DecisionIsolate FinalDecisionKind = "isolate"
	DecisionCancel  FinalDecisionKind = "cancel"
)

// FinalDecision is the single terminal outcome for a task.
type FinalDecision struct {
	TaskID         string
	Kind           FinalDecisionKind
	Version        int64
	EvidenceDigest string
}

// IncubationCredential is the immutable, unique credential issued only on admit.
type IncubationCredential struct {
	TaskID  string `json:"task_id"`
	Number  string `json:"number"`
	Version int64  `json:"version"`
}

// SingleWriter is the single-write barrier for terminal decisions. Only the
// first committed decision wins; it returns the credential when admitting and
// a false won flag when a competing decision already committed.
type SingleWriter interface {
	CommitFinal(ctx context.Context, d FinalDecision) (IncubationCredential, bool, error)
}
