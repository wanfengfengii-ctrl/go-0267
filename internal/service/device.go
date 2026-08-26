package service

import (
	"context"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/evidence"
)

// deviceFunc adapts a plain function to the evidence.DevicePort interface so
// tests can script device behavior without a concrete type.
type deviceFunc func(ctx context.Context, attempt evidence.DeviceAttempt) (string, error)

func (f deviceFunc) Measure(ctx context.Context, attempt evidence.DeviceAttempt) (string, error) {
	return f(ctx, attempt)
}

// defaultDevices returns the healthy device registry used by the smoke test
// and offline acceptance runs. Each device returns a deterministic, in-range
// reading for the evidence kind it is asked to measure.
func defaultDevices() map[string]evidence.DevicePort {
	read := func(kind string) string {
		switch kind {
		case string(catalog.EvidenceEggWeight):
			return "62.50"
		case string(catalog.EvidenceAirCell):
			return "3.20"
		case string(catalog.EvidenceCleanliness):
			return "1"
		case string(catalog.EvidenceColonyCount):
			return "5"
		case string(catalog.EvidenceCtValue):
			return "28.0"
		case string(catalog.EvidenceFumigation):
			return "0.40"
		default:
			return "0"
		}
	}
	return map[string]evidence.DevicePort{
		"dev-culture": deviceFunc(func(ctx context.Context, a evidence.DeviceAttempt) (string, error) {
			return read(string(a.Kind)), nil
		}),
		"dev-reader": deviceFunc(func(ctx context.Context, a evidence.DeviceAttempt) (string, error) {
			return read(string(a.Kind)), nil
		}),
		"dev-scale": deviceFunc(func(ctx context.Context, a evidence.DeviceAttempt) (string, error) {
			return read(string(a.Kind)), nil
		}),
	}
}
