package evidence

import (
	"context"
	"errors"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
)

// CandlingCategory enumerates the mutually exclusive primary candling
// classifications; defects are additive markers reported alongside.
type CandlingCategory string

const (
	CategoryFertile      CandlingCategory = "fertile"
	CategoryInfertile    CandlingCategory = "infertile"
	CategoryCracked      CandlingCategory = "cracked"
	CategoryBloodSpot    CandlingCategory = "blood_spot"
	CategoryContaminated CandlingCategory = "contaminated"
)

// Valid reports whether c is one of the five primary classifications.
func (c CandlingCategory) Valid() bool {
	switch c {
	case CategoryFertile, CategoryInfertile, CategoryCracked,
		CategoryBloodSpot, CategoryContaminated:
		return true
	default:
		return false
	}
}

// Defect is an additive defect marker recorded with a candling entry.
type Defect string

const (
	DefectCrack       Defect = "crack"
	DefectBloodSpot   Defect = "blood_spot"
	DefectContaminate Defect = "contaminate"
)

// Valid reports whether d is a recognized additive defect marker.
func (d Defect) Valid() bool {
	switch d {
	case DefectCrack, DefectBloodSpot, DefectContaminate:
		return true
	default:
		return false
	}
}

// CandlingEntry is one position's current valid candling coverage result.
type CandlingEntry struct {
	TaskID   string           `json:"task_id"`
	SealNo   string           `json:"seal_no"`
	Position int              `json:"position"`
	Category CandlingCategory `json:"category"`
	Defects  []Defect         `json:"defects"`
	Retest   bool             `json:"retest"`
	Version  int              `json:"version"`
}

// PhysicochemicalEvidence is a fixed-point integer measurement for one kind.
type PhysicochemicalEvidence struct {
	TaskID   string               `json:"task_id"`
	SealNo   string               `json:"seal_no"`
	Position int                  `json:"position"`
	Kind     catalog.EvidenceKind `json:"kind"`
	Raw      int64                `json:"raw"`
	Derived  bool                 `json:"derived"`
	Version  int                  `json:"version"`
}

// DeviceFailure enumerates how an external device call may fail; every failure
// only appends a pending-retry attempt and never fabricates a value.
type DeviceFailure string

const (
	FailureRejected DeviceFailure = "rejected"
	FailureDown     DeviceFailure = "down"
	FailureTimeout  DeviceFailure = "timeout"
	FailureFormat   DeviceFailure = "format_error"
)

// DeviceAttempt is the auditable record of one external device invocation.
type DeviceAttempt struct {
	TaskID     string               `json:"task_id"`
	DeviceID   string               `json:"device_id"`
	Kind       catalog.EvidenceKind `json:"kind"`
	Object     string               `json:"object"`
	Generation int                  `json:"generation"`
	Attempt    int                  `json:"attempt"`
	Failure    DeviceFailure        `json:"failure"`
	NextAt     int64                `json:"next_at"`
	Pending    bool                 `json:"pending"`
}

// Device failure sentinels returned by a DevicePort so the caller can record a
// deterministic DeviceAttempt failure category.
var (
	ErrDeviceRejected = errors.New("evidence: device rejected")
	ErrDeviceDown     = errors.New("evidence: device down")
	ErrDeviceTimeout  = errors.New("evidence: device timeout")
	ErrDeviceFormat   = errors.New("evidence: device returned malformed value")
)

// DevicePort isolates external measurement devices behind a port. A call either
// returns the raw decimal text the device produced, or an error that the
// caller persists as a DeviceAttempt; it never releases a lease or fabricates
// a qualified value on failure.
type DevicePort interface {
	Measure(ctx context.Context, attempt DeviceAttempt) (string, error)
}
