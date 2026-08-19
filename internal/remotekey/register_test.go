package remotekey_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/effective"
	"sshc/internal/remotekey"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

const (
	keyLine     = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS fixture@example"
	fingerprint = "SHA256:bytFrSjxj2qRszG8sHhWN+YO3b9vDSU3gQtMorwKpEs"
)

var configSnapshot = []byte("Host bastion\n\tHostName 203.0.113.10\n\tUser ops\n")

// remoteCall は、リモートで走らせた 1 本のコマンドである。
type remoteCall struct {
	alias   string
	command string
	stdin   []byte
}

// scriptedRunner は、リモート実行の継ぎ目を差し替える。
//
// 本物の握手を見るのは internal/sshclient の側である。ここが確かめるのは、
// **何がコマンドとして渡り、何が標準入力を通るか**である。
type scriptedRunner struct {
	calls    []remoteCall
	outputs  []sshclient.Output
	resolved int
}

func (runner *scriptedRunner) run(
	_ context.Context, target sshclient.Target, command string, stdin []byte,
) (sshclient.Output, error) {
	runner.calls = append(runner.calls, remoteCall{alias: target.Alias, command: command, stdin: stdin})
	if len(runner.outputs) == 0 {
		return sshclient.Output{}, nil
	}
	next := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return next, nil
}

func newService(runner *scriptedRunner) remotekey.Service {
	return remotekey.Service{
		Resolve: func(alias string) (sshclient.Target, error) {
			runner.resolved++
			return sshclient.Target{Alias: alias, HostName: "203.0.113.10", Port: "22", User: "ops"}, nil
		},
		Run: runner.run,
	}
}

func TestParsePublicKeyAcceptsOnlyOneValidLine(t *testing.T) {
	key, computed, err := remotekey.ParsePublicKey(keyLine)
	if err != nil {
		t.Fatalf("ParsePublicKey = %v", err)
	}
	if key.Line != keyLine || computed != fingerprint {
		t.Fatalf("key = %#v, fingerprint = %q", key, computed)
	}

	rejected := []string{
		"",
		"ssh-ed25519",
		"ssh-ed25519 not-base64!",
		keyLine + "\nssh-ed25519 AAAA more",
		"rm -rf / AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS comment\rwith-cr",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS comment\x00nul",
	}
	for _, line := range rejected {
		if _, _, err := remotekey.ParsePublicKey(line); !errors.Is(err, remotekey.ErrInvalidPublicKey) {
			t.Errorf("ParsePublicKey(%q) = %v, want ErrInvalidPublicKey", line, err)
		}
	}
}

func TestRegisterProbesThenSendsTheKeyOnStandardInput(t *testing.T) {
	runner := &scriptedRunner{outputs: []sshclient.Output{
		{Stdout: []byte(remotekey.ProbeMarker + "\n")},
		{Stdout: []byte("sshc: added\n")},
	}}
	key, _, err := remotekey.ParsePublicKey(keyLine)
	if err != nil {
		t.Fatal(err)
	}

	result, err := newService(runner).Register(context.Background(), effective.Report{}, configSnapshot, "bastion", key, false)
	if err != nil {
		t.Fatalf("Register = %v", err)
	}
	if result.Outcome != remotekey.RegistrationAdded {
		t.Fatalf("result = %#v", result)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v", runner.calls)
	}

	probe := runner.calls[0]
	if probe.command != remotekey.ProbeCommand || probe.alias != "bastion" {
		t.Errorf("probe = %#v", probe)
	}
	if len(probe.stdin) != 0 {
		t.Errorf("the probe sent %q on standard input", probe.stdin)
	}

	register := runner.calls[1]
	if register.command != remotekey.Routine {
		t.Errorf("registration command = %q", register.command)
	}
	// **公開鍵は標準入力を通る。コマンドには決して乗らない。**
	if string(register.stdin) != keyLine+"\n" {
		t.Errorf("stdin = %q, want the key line", register.stdin)
	}
	if strings.Contains(register.command, "AAAAC3Nza") || strings.Contains(register.command, "fixture@example") {
		t.Fatalf("the command carried key material: %q", register.command)
	}
	if strings.Contains(remotekey.Routine, "fixture@example") {
		t.Error("the remote routine must never contain caller input")
	}
	// **行き先は一度だけ決める。** 二度解決すると、その間に設定を書き換えた者が
	// 二本目の行き先を変えられる。
	if runner.resolved != 1 {
		t.Errorf("the destination was resolved %d times, want once", runner.resolved)
	}
}

func TestRegisterReportsAnExistingKeyAndAnUnsupportedRemote(t *testing.T) {
	key, _, err := remotekey.ParsePublicKey(keyLine)
	if err != nil {
		t.Fatal(err)
	}

	existing := &scriptedRunner{outputs: []sshclient.Output{
		{Stdout: []byte(remotekey.ProbeMarker + "\n")},
		{Stdout: []byte("sshc: already-present\n")},
	}}
	result, err := newService(existing).Register(context.Background(), effective.Report{}, configSnapshot, "bastion", key, false)
	if err != nil {
		t.Fatalf("Register = %v", err)
	}
	if result.Outcome != remotekey.RegistrationExisting {
		t.Fatalf("result = %#v", result)
	}

	unsupported := &scriptedRunner{outputs: []sshclient.Output{
		{Stdout: []byte("Windows PowerShell\n"), ExitCode: 0},
	}}
	if _, err := newService(unsupported).Register(context.Background(), effective.Report{}, configSnapshot, "bastion", key, false); !errors.Is(err, remotekey.ErrUnsupportedRemote) {
		t.Fatalf("Register = %v, want ErrUnsupportedRemote", err)
	}
	if len(unsupported.calls) != 1 {
		t.Fatal("an unsupported remote still received the registration routine")
	}
}

func TestRegisterRefusesUntilExecutableDirectivesAreAcknowledged(t *testing.T) {
	runner := &scriptedRunner{}
	key, _, err := remotekey.ParsePublicKey(keyLine)
	if err != nil {
		t.Fatal(err)
	}
	report := effective.Report{Directives: []effective.Executable{
		{Keyword: "ProxyCommand", Command: "/usr/bin/nc %h %p", OnConnect: true},
	}}

	if _, err := newService(runner).Register(context.Background(), report, configSnapshot, "bastion", key, false); !errors.Is(err, remotekey.ErrNotAcknowledged) {
		t.Fatalf("Register = %v, want ErrNotAcknowledged", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("a refused registration reached the remote")
	}

	if _, err := newService(runner).Register(context.Background(), effective.Report{}, configSnapshot, "bad alias", key, false); !errors.Is(err, validate.ErrUnsafeAlias) {
		t.Fatalf("Register = %v, want ErrUnsafeAlias", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("an unsafe alias reached the remote")
	}
}

func TestPlanDescribesExactlyWhatWillHappen(t *testing.T) {
	key, computed, err := remotekey.ParsePublicKey(keyLine)
	if err != nil {
		t.Fatal(err)
	}
	plan := newService(&scriptedRunner{}).Plan("bastion", key, computed, "ops", "203.0.113.10", "2222", "openssh")

	if !plan.Supported || plan.RemotePath != "~/.ssh/authorized_keys" {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Routine != remotekey.Routine || plan.KeyLine != keyLine || plan.Fingerprint != fingerprint {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.User != "ops" || plan.Hostname != "203.0.113.10" || plan.Port != "2222" || plan.ValuesFrom != "openssh" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.Manual) == 0 || !strings.Contains(strings.Join(plan.Manual, "\n"), "authorized_keys") {
		t.Errorf("manual steps = %#v", plan.Manual)
	}
}
