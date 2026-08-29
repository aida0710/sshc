package terminal

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func agentSequence(json string, terminator string) string {
	return string(agentOSCPrefix) + base64.RawURLEncoding.EncodeToString([]byte(json)) + terminator
}

func TestAgentDecoderHandlesEveryChunkBoundary(t *testing.T) {
	sequence := agentSequence(`{"v":1,"agent":"codex","event":"working","session":"thread_123","name":"API修正","seq":1}`, "\x1b\\")
	input := "before" + sequence + "after"
	for split := 0; split <= len(input); split++ {
		var events []agentEvent
		decoder := newAgentDecoder(func(event agentEvent) { events = append(events, event) })
		visible := append(decoder.Write([]byte(input[:split])), decoder.Write([]byte(input[split:]))...)
		visible = append(visible, decoder.Flush()...)
		if got := string(visible); got != "beforeafter" {
			t.Fatalf("split %d: visible=%q", split, got)
		}
		if len(events) != 1 || events[0].Agent != AgentCodex || events[0].Session != "thread_123" {
			t.Fatalf("split %d: events=%+v", split, events)
		}
	}
}

func TestAgentDecoderAcceptsBELAndPreservesOtherOSC(t *testing.T) {
	other := "\x1b]0;ordinary title\a"
	target := agentSequence(`{"v":1,"agent":"claude","event":"ready","session":"abc-123","seq":2}`, "\a")
	var events []agentEvent
	decoder := newAgentDecoder(func(event agentEvent) { events = append(events, event) })
	visible := append(decoder.Write([]byte(other+target+"tail")), decoder.Flush()...)
	if got := string(visible); got != other+"tail" {
		t.Fatalf("visible=%q", got)
	}
	if len(events) != 1 || events[0].Agent != AgentClaude {
		t.Fatalf("events=%+v", events)
	}
}

func TestAgentDecoderDropsMalformedAndOversizedTarget(t *testing.T) {
	malformed := string(agentOSCPrefix) + "%%%\a"
	oversized := string(agentOSCPrefix) + strings.Repeat("a", MaxAgentPayload+1) + "\x1b\\"
	decoder := newAgentDecoder(func(agentEvent) { t.Fatal("invalid event was accepted") })
	visible := append(decoder.Write([]byte("a"+malformed+"b"+oversized+"c")), decoder.Flush()...)
	if got := string(visible); got != "abc" {
		t.Fatalf("visible=%q", got)
	}
}

func TestAgentResumeArgvIsAdapterOwned(t *testing.T) {
	tests := []struct {
		kind AgentKind
		want string
	}{
		{AgentClaude, "claude --resume safe-id"},
		{AgentCodex, "codex resume safe-id"},
		{AgentOpenCode, "opencode --session safe-id"},
	}
	for _, test := range tests {
		executable, arguments, err := AgentResumeArgv(test.kind, "safe-id")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(append([]string{executable}, arguments...), " "); got != test.want {
			t.Fatalf("%s: got %q", test.kind, got)
		}
	}
	for _, reference := range []string{"", "x;rm", "x y", strings.Repeat("a", 257)} {
		if _, _, err := AgentResumeArgv(AgentCodex, reference); err == nil {
			t.Fatalf("unsafe reference %q was accepted", reference)
		}
	}
}

func TestAgentDisplayRemovesControlAndDirectionOverrides(t *testing.T) {
	if got := cleanAgentDisplay(" API\n修正\u202e.txt ", MaxTitle); got != "API修正.txt" {
		t.Fatalf("cleaned display=%q", got)
	}
}

func FuzzAgentDecoderPreservesOrdinaryText(f *testing.F) {
	for _, seed := range []string{"plain", "\x1b]0;title\a", "\x1b", string(agentOSCPrefix)} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if strings.Contains(string(input), string(agentOSCPrefix)) {
			t.Skip()
		}
		decoder := newAgentDecoder(nil)
		var output []byte
		for _, value := range input {
			output = append(output, decoder.Write([]byte{value})...)
		}
		output = append(output, decoder.Flush()...)
		if string(output) != string(input) {
			t.Fatalf("input=%s output=%s", fmt.Sprintf("%x", input), fmt.Sprintf("%x", output))
		}
	})
}
