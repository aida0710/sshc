package terminal

import (
	"context"
	"testing"
	"time"
)

func agentResumeStub(context.Context, Size, AgentKind, string) (Process, error) {
	return nil, nil
}

func TestAgentObservationDrivesTitleAndSignalsWithoutExposingReference(t *testing.T) {
	started := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{
		kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka",
		titleSource: TitleConnection, generation: 1, now: func() time.Time { return started.Add(4 * time.Second) },
		resume: agentResumeStub,
	}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentCodex, Event: "working", Session: "thread_123",
		Name: "API認証の修正", Seq: 1,
	}, started)
	working := session.View()
	if working.Title != "API認証の修正" || working.Presentation.TitleSource != TitleAgent {
		t.Fatalf("working presentation=%+v", working.Presentation)
	}
	if working.Agent == nil || working.Agent.State != AgentWorking || !working.Agent.Resumable {
		t.Fatalf("working agent=%+v", working.Agent)
	}

	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentCodex, Event: "ready", Session: "thread_123", Seq: 2,
	}, started.Add(4*time.Second))
	ready := session.View()
	if ready.Agent == nil || ready.Agent.LastSignal == nil || ready.Agent.LastSignal.Kind != AgentSignalCompleted {
		t.Fatalf("ready agent=%+v", ready.Agent)
	}
	if ready.Agent.SignalVersion != 1 {
		t.Fatalf("signal version=%d", ready.Agent.SignalVersion)
	}
}

func TestPinnedTitleWinsAndUnpinRecomputes(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka", titleSource: TitleConnection, generation: 1, resume: agentResumeStub}
	if err := session.Rename("固定名"); err != nil {
		t.Fatal(err)
	}
	session.acceptAgentEvent(1, agentEvent{Version: 1, Agent: AgentClaude, Event: "working", Session: "session-1", Name: "自動名", Seq: 1}, now)
	if got := session.View(); got.Title != "固定名" || !got.Presentation.TitlePinned {
		t.Fatalf("pinned view=%+v", got)
	}
	session.UnpinTitle()
	if got := session.View(); got.Title != "自動名" || got.Presentation.TitleSource != TitleAgent {
		t.Fatalf("unpinned view=%+v", got)
	}
}

func TestAgentCandidateSurvivesGenerationChangeButDelayedEventDoesNot(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka", titleSource: TitleConnection, generation: 1, resume: agentResumeStub}
	session.acceptAgentEvent(1, agentEvent{Version: 1, Agent: AgentOpenCode, Event: "working", Session: "ses_123", Name: "調査", Seq: 1}, now)
	session.mutex.Lock()
	session.advanceAgentGenerationLocked()
	session.generation++
	session.mutex.Unlock()
	session.acceptAgentEvent(1, agentEvent{Version: 1, Agent: AgentOpenCode, Event: "attention", Session: "ses_123", Name: "古い名前", Seq: 2}, now.Add(time.Second))
	view := session.View()
	if view.Agent == nil || view.Agent.State != AgentUnknown || !view.Agent.Resumable {
		t.Fatalf("candidate agent=%+v", view.Agent)
	}
	if view.Title != "調査" || view.Presentation.TitleSource != TitleCandidate {
		t.Fatalf("candidate presentation=%+v", view.Presentation)
	}
}

func TestNewSessionOfSameAdapterMayRestartSequence(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka", titleSource: TitleConnection, generation: 1}
	session.acceptAgentEvent(1, agentEvent{Version: 1, Agent: AgentCodex, Event: "working", Session: "thread-old", Name: "古い作業", Seq: 20}, now)
	session.acceptAgentEvent(1, agentEvent{Version: 1, Agent: AgentCodex, Event: "ready", Session: "thread-new", Name: "新しい作業", Seq: 1}, now.Add(time.Second))
	view := session.View()
	if view.Title != "新しい作業" || view.Agent == nil || view.Agent.State != AgentReady {
		t.Fatalf("new identity was mistaken for an old sequence: %+v", view)
	}
}

func TestContinuingAgentEventKeepsOmittedMetadata(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka", titleSource: TitleConnection, generation: 1}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentClaude, Event: "ready", Session: "session-1",
		CWD: "/workspace", Model: "claude-sonnet-4-6", Seq: 1,
	}, now)
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentClaude, Event: "working", Session: "session-1", Seq: 2,
	}, now.Add(time.Second))

	view := session.View()
	if view.Agent == nil || view.Agent.CWD != "/workspace" || view.Agent.Model != "claude-sonnet-4-6" {
		t.Fatalf("omitted metadata was not preserved: %+v", view.Agent)
	}
}

func TestAgentExitBecomesAnUnknownResumeCandidate(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{
		kind: KindSSH, alias: "osaka", title: "osaka", fallbackTitle: "osaka",
		titleSource: TitleConnection, generation: 1, resume: agentResumeStub,
	}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentClaude, Event: "ready", Session: "session-1", Name: "修正作業", Seq: 1,
	}, now)

	session.finish(ExitInfo{Code: 0, At: now.Add(time.Second)})

	view := session.View()
	if view.Agent == nil || view.Agent.State != AgentUnknown || !view.Agent.Resumable {
		t.Fatalf("exited agent=%+v", view.Agent)
	}
	if view.Title != "修正作業" || view.Presentation.TitleSource != TitleCandidate {
		t.Fatalf("exited presentation=%+v", view.Presentation)
	}
}

func TestLocalAgentObservationDoesNotClaimUnsupportedResume(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	session := &Session{
		kind: KindShell, title: "zsh", fallbackTitle: "zsh",
		titleSource: TitleFallback, generation: 1,
	}
	session.acceptAgentEvent(1, agentEvent{
		Version: 1, Agent: AgentCodex, Event: "ready", Session: "thread-1", Seq: 1,
	}, now)

	view := session.View()
	if view.Agent == nil || view.Agent.Resumable {
		t.Fatalf("local agent must remain observable without claiming resume support: %+v", view.Agent)
	}
}
