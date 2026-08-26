package arbitration

import (
	"errors"

	"github.com/hatchseal/hatchseal-breeder-egg-incubation-gate/internal/catalog"
)

// ErrEvidenceIncomplete is returned when a command needs closed evidence but a
// required collection stage is not yet complete.
var ErrEvidenceIncomplete = errors.New("arbitration: evidence not closed")

// Within reports whether a fixed-point value satisfies the closed threshold
// bound, honouring the inclusive/exclusive flags uniformly (domain rule 5).
func Within(value int64, t catalog.Threshold) bool {
	if t.InclusiveMin {
		if value < t.Min {
			return false
		}
	} else if value <= t.Min {
		return false
	}
	if t.InclusiveMax {
		if value > t.Max {
			return false
		}
	} else if value >= t.Max {
		return false
	}
	return true
}

// EvidenceClosure records which collection stages have been completed. A task
// may advance to independent review only when every stage is closed.
type EvidenceClosure struct {
	CandlingComplete  bool
	SwabSealed        bool
	CultureComplete   bool
	RapidTestComplete bool
	PhysicochemicalOK bool
	RetestsResolved   bool
}

// Complete reports whether all evidence has been closed and retests resolved.
func (c EvidenceClosure) Complete() bool {
	return c.CandlingComplete && c.SwabSealed && c.CultureComplete &&
		c.RapidTestComplete && c.PhysicochemicalOK && c.RetestsResolved
}

// PollutionTrigger enumerates the conditions that may open a retest case.
type PollutionTrigger string

const (
	TriggerLowViability     PollutionTrigger = "low_viability"
	TriggerCrackExcess      PollutionTrigger = "crack_excess"
	TriggerSuspectPositive  PollutionTrigger = "suspect_positive"
	TriggerCulturePollution PollutionTrigger = "culture_pollution"
	TriggerSampleDivergence PollutionTrigger = "sample_divergence"
)

// RetestVerdict is the outcome of a resolved retest case.
type RetestVerdict string

const (
	VerdictContaminated RetestVerdict = "contaminated"
	VerdictClean        RetestVerdict = "clean"
	VerdictPending      RetestVerdict = "pending"
)
