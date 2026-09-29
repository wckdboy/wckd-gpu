package session

import "fmt"

// Phase is the P1 session lifecycle.
//
//	pending → hydrating → ready → draining → terminated
//
// failed is the escape hatch for provision errors and for a failed session
// that still needs a drain. Same-state transitions are no-ops.
type Phase string

const (
	PhasePending    Phase = "pending"
	PhaseHydrating  Phase = "hydrating"
	PhaseReady      Phase = "ready"
	PhaseDraining   Phase = "draining"
	PhaseTerminated Phase = "terminated"
	PhaseFailed     Phase = "failed"
)

// AllPhases is the closed set of phases. Tests use it to catch a new phase
// that is missing from the transition table.
var AllPhases = []Phase{
	PhasePending,
	PhaseHydrating,
	PhaseReady,
	PhaseDraining,
	PhaseTerminated,
	PhaseFailed,
}

// allowed lists outbound transitions. Terminal phases have no outbound edges;
// repeating the current phase is handled before this table is consulted.
var allowed = map[Phase]map[Phase]bool{
	PhasePending:   {PhaseHydrating: true, PhaseDraining: true, PhaseFailed: true},
	PhaseHydrating: {PhaseReady: true, PhaseDraining: true, PhaseFailed: true},
	PhaseReady:     {PhaseDraining: true, PhaseFailed: true},
	PhaseDraining:  {PhaseTerminated: true, PhaseFailed: true},
	PhaseFailed:    {PhaseDraining: true},
}

// Active reports whether the session can still be billed or drained.
func (p Phase) Active() bool {
	switch p {
	case PhasePending, PhaseHydrating, PhaseReady, PhaseDraining:
		return true
	case PhaseTerminated, PhaseFailed:
		return false
	default:
		return false
	}
}

// Transition checks one step of the state machine.
func Transition(from, to Phase) error {
	if from == to {
		return nil
	}
	if allowed[from][to] {
		return nil
	}
	return fmt.Errorf("invalid session transition %s -> %s", from, to)
}
