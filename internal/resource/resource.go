// Package resource models the 蛋盘样本与资源占用账簿 (tray sample and resource
// occupancy ledger): tray seals with their ordered positions, blind samples,
// and the four families of concurrent leases guarded by unique constraints,
// lease generation and logical expiry.
package resource

import "context"

// TraySeal binds a seal number to an ordered set of positions.
type TraySeal struct {
	SealNo    string
	Positions []TrayPosition
}

// TrayPosition is a single position within a tray seal's ordered grid.
type TrayPosition struct {
	Position int
}

// BlindSample carries a blind code's ciphertext/digest and reveal state.
type BlindSample struct {
	Code          string
	Digest        string
	Revealed      bool
	RevealVersion int
}

// LeaseType enumerates the reservable resource families. The four physical
// resources (incubator slot, candling window, culture well, rapid-test well)
// are joined by three binding keys (batch number, tray seal, blind code) which
// are likewise globally unique among open tasks per domain rule 2.
type LeaseType string

const (
	LeaseBatchNo        LeaseType = "batch_no"
	LeaseTraySeal       LeaseType = "tray_seal"
	LeaseBlindCode      LeaseType = "blind_code"
	LeaseIncubatorSlot  LeaseType = "incubator_slot"
	LeaseCandlingWindow LeaseType = "candling_window"
	LeaseCultureWell    LeaseType = "culture_well"
	LeaseRapidTestWell  LeaseType = "rapid_test_well"
)

// Binding reports whether the lease type is an immutable binding key. Binding
// keys may never be exchanged or rebound once locked (domain rule 2).
func (t LeaseType) Binding() bool {
	switch t {
	case LeaseBatchNo, LeaseTraySeal, LeaseBlindCode:
		return true
	default:
		return false
	}
}

// ResourceLease records one held resource, its owning task generation, and the
// logical acquisition/expiry instants. Open leases are unique per type and key.
type ResourceLease struct {
	Type          LeaseType
	ResourceKey   string
	TaskID        string
	Generation    int
	AcquiredAt    int64
	ExpiresAt     int64
	ReleaseReason string
}

// Repository acquires and releases leases. Acquire must fail atomically when a
// conflicting open lease exists for the same type and key; Release is idempotent.
type Repository interface {
	Acquire(ctx context.Context, lease ResourceLease) error
	Release(ctx context.Context, leaseType LeaseType, resourceKey string) error
	Active(ctx context.Context, taskID string) ([]ResourceLease, error)
}
