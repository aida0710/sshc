package terminal

import (
	"testing"
	"time"
)

func TestControlStateUsesOnlyExplicitLifecycleState(t *testing.T) {
	now := time.Now()
	session := &Session{
		generation: 1, state: StateConnected, buffer: NewRing(32),
		now: func() time.Time { return now },
	}
	if got, _ := session.ReadControl(0, 0); got.State != ControlConnected {
		t.Fatalf("initial state = %q", got.State)
	}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentCodex, Event: "working", Session: "thread-1", Seq: 1,
	}, now)
	if got, _ := session.ReadControl(0, 0); got.State != ControlAgentWorking {
		t.Fatalf("working state = %q", got.State)
	}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentCodex, Event: "ended", Session: "thread-1", Seq: 2,
	}, now.Add(time.Second))
	if got, _ := session.ReadControl(0, 0); got.State != ControlAgentEnded {
		t.Fatalf("ended state = %q", got.State)
	}
	session.generation++
	if got, _ := session.ReadControl(0, 0); got.State != ControlConnected {
		t.Fatalf("old ended event leaked into generation 2: %q", got.State)
	}
}
