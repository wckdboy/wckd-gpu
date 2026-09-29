package session

import "testing"

func TestTransitions(t *testing.T) {
	ok := [][2]Phase{
		{PhasePending, PhaseHydrating},
		{PhasePending, PhaseDraining},
		{PhasePending, PhaseFailed},
		{PhaseHydrating, PhaseReady},
		{PhaseHydrating, PhaseDraining},
		{PhaseHydrating, PhaseFailed},
		{PhaseReady, PhaseDraining},
		{PhaseReady, PhaseFailed},
		{PhaseDraining, PhaseTerminated},
		{PhaseDraining, PhaseFailed},
		{PhaseFailed, PhaseDraining},
		{PhaseReady, PhaseReady},
		{PhaseTerminated, PhaseTerminated},
	}
	for _, pair := range ok {
		if err := Transition(pair[0], pair[1]); err != nil {
			t.Errorf("%s -> %s: %v", pair[0], pair[1], err)
		}
	}

	bad := [][2]Phase{
		{PhasePending, PhaseReady},
		{PhasePending, PhaseTerminated},
		{PhaseHydrating, PhaseTerminated},
		{PhaseReady, PhaseTerminated},
		{PhaseReady, PhasePending},
		{PhaseDraining, PhaseReady},
		{PhaseTerminated, PhasePending},
		{PhaseTerminated, PhaseDraining},
		{PhaseFailed, PhaseReady},
		{PhaseFailed, PhaseTerminated},
	}
	for _, pair := range bad {
		if err := Transition(pair[0], pair[1]); err == nil {
			t.Errorf("expected %s -> %s to fail", pair[0], pair[1])
		}
	}
}

func TestEveryPhaseIsKnown(t *testing.T) {
	seen := map[Phase]bool{}
	for _, p := range AllPhases {
		if seen[p] {
			t.Fatalf("duplicate %s", p)
		}
		seen[p] = true
		if p != PhaseTerminated && p != PhaseFailed {
			if _, ok := allowed[p]; !ok {
				t.Errorf("non-terminal %s missing from transition table", p)
			}
			if !allowed[p][PhaseDraining] && p != PhaseDraining {
				t.Errorf("%s cannot reach draining", p)
			}
		}
		_ = p.Active()
	}
	if len(allowed[PhaseTerminated]) != 0 || allowed[PhaseFailed][PhaseTerminated] {
		t.Fatal("terminal phases should not jump to terminated without draining")
	}
	for from := range allowed {
		if !seen[from] {
			t.Errorf("unknown phase %s in table", from)
		}
	}
}
