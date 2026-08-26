package resource

import "errors"

// ErrNotExchangeable is returned when a caller tries to exchange a binding
// lease (batch number, tray seal or blind code) instead of a window.
var ErrNotExchangeable = errors.New("resource: lease type is not exchangeable")

// ErrLeaseConflict is returned when a lease's type/key is already held by an
// open task (the unique index maps to a stable business error).
var ErrLeaseConflict = errors.New("resource: lease already held")

// Exchange swaps an existing open lease for a target key in a single atomic
// step. Binding keys are rejected outright; only the reservable window/slot
// families may be exchanged. The old lease is released only after the target
// is validated, so a failed exchange preserves the original occupancy.
type Exchange struct {
	Type        LeaseType
	FromKey     string
	ToKey       string
	TaskID      string
	Generation  int
	LogicalTime int64
}

// Validate checks the exchange request before any mutation.
func (x Exchange) Validate() error {
	if x.Type.Binding() {
		return ErrNotExchangeable
	}
	if x.FromKey == "" || x.ToKey == "" || x.FromKey == x.ToKey {
		return errors.New("resource: invalid exchange keys")
	}
	if x.TaskID == "" {
		return errors.New("resource: missing task id")
	}
	return nil
}
