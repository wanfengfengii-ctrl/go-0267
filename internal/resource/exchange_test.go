package resource

import (
	"errors"
	"testing"
)

func TestLeaseTypeBinding(t *testing.T) {
	for _, lt := range []LeaseType{LeaseBatchNo, LeaseTraySeal, LeaseBlindCode} {
		if !lt.Binding() {
			t.Errorf("%s should be a binding lease", lt)
		}
	}
	for _, lt := range []LeaseType{LeaseIncubatorSlot, LeaseCandlingWindow, LeaseCultureWell, LeaseRapidTestWell} {
		if lt.Binding() {
			t.Errorf("%s should not be a binding lease", lt)
		}
	}
}

func TestExchangeValidateRejectsBinding(t *testing.T) {
	x := Exchange{Type: LeaseBatchNo, FromKey: "a", ToKey: "b", TaskID: "t"}
	if !errors.Is(x.Validate(), ErrNotExchangeable) {
		t.Error("binding lease exchange should be rejected")
	}
}

func TestExchangeValidateKeys(t *testing.T) {
	x := Exchange{Type: LeaseIncubatorSlot, FromKey: "a", ToKey: "a", TaskID: "t"}
	if err := x.Validate(); err == nil {
		t.Error("same from/to key should be rejected")
	}
	x = Exchange{Type: LeaseIncubatorSlot, FromKey: "a", ToKey: "b"}
	if err := x.Validate(); err == nil {
		t.Error("missing task id should be rejected")
	}
	x = Exchange{Type: LeaseIncubatorSlot, FromKey: "a", ToKey: "b", TaskID: "t"}
	if err := x.Validate(); err != nil {
		t.Errorf("valid exchange rejected: %v", err)
	}
}
