package main

import (
	"strings"
	"testing"
	"time"

	"sshc/internal/streamrun"
)

func TestBuildTransportScriptAcceptsStrictVersionedDocument(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Script = "-"
	script, err := buildTransportScript(called, strings.NewReader(`{
		"version": 1,
		"steps": [
			{"expect": "login: ", "timeout": "2s"},
			{"sendEnv": "CONSOLE_USER", "lineEnding": "lf"},
			{"send": "", "lineEnding": "crlf"},
			{"readFor": "50ms"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Steps) != 4 || script.Steps[0].Timeout != 2*time.Second || script.Steps[0].LineEnding != streamrun.EndingNone {
		t.Fatalf("script = %#v", script)
	}
	if script.Steps[1].SendEnv != "CONSOLE_USER" || script.Steps[1].LineEnding != streamrun.EndingLF {
		t.Fatalf("sendEnv step = %#v", script.Steps[1])
	}
	if script.Steps[2].Send == nil || *script.Steps[2].Send != "" || script.Steps[2].LineEnding != streamrun.EndingCRLF {
		t.Fatalf("empty send step = %#v", script.Steps[2])
	}
	if script.Steps[3].ReadFor != 50*time.Millisecond || script.Steps[3].LineEnding != streamrun.EndingNone {
		t.Fatalf("readFor step = %#v", script.Steps[3])
	}
}

func TestBuildTransportScriptRejectsUnknownFieldsAndTrailingDocuments(t *testing.T) {
	called := defaultTransportInvocation(transportTelnet, true)
	called.Script = "-"
	for _, document := range []string{
		`{"version":1,"steps":[],"unexpected":true}`,
		`{"version":1,"steps":[]} {"version":1,"steps":[]}`,
		`{"version":2,"steps":[]}`,
	} {
		if _, err := buildTransportScript(called, strings.NewReader(document)); err == nil {
			t.Fatalf("accepted %s", document)
		}
	}
}
