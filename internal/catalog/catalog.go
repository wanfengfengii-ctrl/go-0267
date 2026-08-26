// Package catalog models the 种禽舍与入孵规则目录 (house and incubation rule
// directory): the reference data a locked task snapshots, including houses,
// collection shifts, fumigation batches, reservable slots/windows, devices,
// personnel qualifications, and versioned precision/threshold rule sets.
package catalog

import (
	"context"
	"time"
)

// CatalogHouse is a parent-stock house with an effective validity window.
type CatalogHouse struct {
	ID        string
	Code      string
	Name      string
	ValidFrom time.Time
	ValidTo   time.Time
}

// CollectionShift is an egg-collection shift associated with a house.
type CollectionShift struct {
	ID        string
	HouseID   string
	Code      string
	ValidFrom time.Time
	ValidTo   time.Time
}

// FumigationBatch is a fumigation cabinet batch whose digest is versioned.
// A task may only advance when its snapshot digest matches the current version.
type FumigationBatch struct {
	ID      string
	Digest  string
	Version int
}

// IncubatorSlot is a reservable incubator time slot.
type IncubatorSlot struct {
	ID        string
	Code      string
	ValidFrom time.Time
	ValidTo   time.Time
}

// CandlingWindow is a reservable candling window.
type CandlingWindow struct {
	ID   string
	Code string
}

// DeviceKind enumerates the external device families used during collection.
type DeviceKind string

const (
	DeviceCandler     DeviceKind = "candler"
	DeviceCultureBox  DeviceKind = "culture_box"
	DevicePlateReader DeviceKind = "rapid_plate_reader"
	DeviceScale       DeviceKind = "scale"
)

// Device is an external measurement device identified by kind.
type Device struct {
	ID   string
	Kind DeviceKind
	Code string
}

// Role enumerates the qualified roles a person may hold in the flow.
type Role string

const (
	RoleReceiver   Role = "receiver"
	RoleReviewer   Role = "reviewer"
	RoleAuthorizer Role = "authorizer"
)

// PersonQualification records the roles a person may currently hold.
type PersonQualification struct {
	PersonID  string
	Roles     []Role
	ValidFrom time.Time
	ValidTo   time.Time
}

// EvidenceKind enumerates the fixed-point evidence measurements governed by a
// rule set's precision and threshold definitions.
type EvidenceKind string

const (
	EvidenceEggWeight   EvidenceKind = "egg_weight"
	EvidenceAirCell     EvidenceKind = "air_cell_height"
	EvidenceCleanliness EvidenceKind = "cleanliness"
	EvidenceColonyCount EvidenceKind = "colony_count"
	EvidenceCtValue     EvidenceKind = "ct_value"
	EvidenceFumigation  EvidenceKind = "fumigation_residue"
)

// Threshold is a closed business bound over a signed fixed-point integer.
type Threshold struct {
	Min          int64
	Max          int64
	InclusiveMin bool
	InclusiveMax bool
}

// RuleSetVersion holds fixed decimal precisions and inspection thresholds for
// a referenceable rules version.
type RuleSetVersion struct {
	Version    int
	Precisions map[EvidenceKind]int
	Thresholds map[EvidenceKind]Threshold
}

// Reader exposes the catalog lookups a lock command performs to validate
// source matching, digest freshness, and rule snapshots. Implementations read
// committed reference data; a locked task only ever consumes snapshots.
type Reader interface {
	House(ctx context.Context, id string) (CatalogHouse, error)
	Shift(ctx context.Context, id string) (CollectionShift, error)
	Fumigation(ctx context.Context, id string) (FumigationBatch, error)
	Slot(ctx context.Context, id string) (IncubatorSlot, error)
	Window(ctx context.Context, id string) (CandlingWindow, error)
	Qualification(ctx context.Context, personID string) (PersonQualification, error)
	RuleSet(ctx context.Context, version int) (RuleSetVersion, error)
	Device(ctx context.Context, id string) (Device, error)
}
