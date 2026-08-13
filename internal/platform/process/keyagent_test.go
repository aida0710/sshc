package process_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/process"
)

// recordingRunner は、実行されたはずのコマンドを記録する。このパッケージのどの
// テストも、本物の ssh-add を起動せず、本物のエージェントやキーチェーンに触れない。
type recordingRunner struct {
	commands []platform.Command
	outputs  []platform.Output
	err      error
}

func (recorder *recordingRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	recorder.commands = append(recorder.commands, command)
	if recorder.err != nil {
		return platform.Output{}, recorder.err
	}
	if len(recorder.outputs) == 0 {
		return platform.Output{}, nil
	}
	output := recorder.outputs[0]
	recorder.outputs = recorder.outputs[1:]
	return output, nil
}

// toolchainStub は platform.Toolchain を満たす最小のスタブ。KeyAgent が呼ぶのは
// KeyAdd だけなので、他の 3 メソッドは呼ばれることを想定せず、値を返すだけで
// よい。
type toolchainStub struct {
	keyAddPath string
	keyAddErr  error
}

func (t toolchainStub) SSH() (string, error)    { return "", nil }
func (t toolchainStub) KeyGen() (string, error) { return "", nil }
func (t toolchainStub) KeyAdd() (string, error) { return t.keyAddPath, t.keyAddErr }

// installedToolchain は、ssh-add の絶対パスを直接返すスタブ。これにより
// ここのどのテストも、このマシンにたまたま入っている OpenSSH に依存しない。
func installedToolchain() toolchainStub {
	return toolchainStub{keyAddPath: "/usr/bin/ssh-add"}
}

// agentLookup は、ssh-add を、このアプリケーションが供給する標準入力を読ませる
// 代わりにユーザーの選んだ外部プログラムへ向け直してしまう askpass 系の変数を、
// 意図的に差し出す。このファイルのすべてのテストは、その敵対的な環境に対して
// 走る。
func agentLookup(name string) (string, bool) {
	switch name {
	case "SSH_AUTH_SOCK":
		return "/tmp/fake-agent.sock", true
	case "HOME":
		return "/Users/example", true
	case "PATH":
		return "/usr/bin:/bin", true
	case "SSH_ASKPASS":
		return "/opt/attacker/askpass", true
	case "SSH_ASKPASS_REQUIRE":
		return "force", true
	case "DISPLAY":
		return ":0", true
	default:
		return "", false
	}
}

// assertScrubbedEnvironment は、このアダプタ全体の要点である。子プロセスは、
// askpass プログラムへ向け直されえない、置き換えられた環境を受け取らねばならない。
func assertScrubbedEnvironment(t *testing.T, command platform.Command) {
	t.Helper()
	if command.Env == nil {
		t.Fatalf("Env is nil, so the child inherits this process's environment")
	}
	for _, forbidden := range []string{"SSH_ASKPASS=", "SSH_ASKPASS_REQUIRE=", "DISPLAY="} {
		for _, entry := range command.Env {
			if strings.HasPrefix(entry, forbidden) {
				t.Fatalf("Env carried %q: %#v", forbidden, command.Env)
			}
		}
	}
	if strings.Contains(strings.Join(command.Env, "\n"), "/opt/attacker/askpass") {
		t.Fatalf("Env carried the askpass program: %#v", command.Env)
	}
	socket := false
	for _, entry := range command.Env {
		if entry == "SSH_AUTH_SOCK=/tmp/fake-agent.sock" {
			socket = true
		}
	}
	if !socket {
		t.Fatalf("Env = %#v, want the agent socket so the agent stays reachable", command.Env)
	}
}

// Keychain 経路は廃止した。パスフレーズの保存先は自前の vault ひとつであり、
// ssh-add に二つ目の保管場所を持たせない。--apple-use-keychain が復活すれば、
// 鍵を移動したときに絶対パスで識別された項目が壊れる問題も一緒に戻ってくる。
func TestAddNeverAsksSshAddToStoreThePassphrase(t *testing.T) {
	recorder := &recordingRunner{}
	agent := process.NewKeyAgent(recorder, installedToolchain(), agentLookup)

	err := agent.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath:  "/home/u/.ssh/id_ed25519",
		Passphrase:      []byte("secret"),
		LifetimeSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range recorder.commands {
		for _, argument := range command.Arguments {
			if strings.Contains(argument, "keychain") {
				t.Fatalf("argv がキーチェーンに触れている: %#v", command.Arguments)
			}
		}
	}
}

func TestKeyAgentAddSendsThePassphraseOnlyOnStandardInput(t *testing.T) {
	recorder := &recordingRunner{outputs: []platform.Output{{}}}
	agent := process.NewKeyAgent(recorder, installedToolchain(), agentLookup)

	err := agent.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath:  "/Users/example/.ssh/id_work",
		Passphrase:      []byte("correct horse"),
		LifetimeSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if len(recorder.commands) != 1 {
		t.Fatalf("commands = %#v, want one", recorder.commands)
	}
	command := recorder.commands[0]
	if command.Path != "/usr/bin/ssh-add" {
		t.Errorf("Path = %q, want /usr/bin/ssh-add", command.Path)
	}
	want := []string{"-t", "3600", "/Users/example/.ssh/id_work"}
	if strings.Join(command.Arguments, " ") != strings.Join(want, " ") {
		t.Fatalf("Arguments = %#v, want %#v", command.Arguments, want)
	}
	for _, argument := range command.Arguments {
		if strings.Contains(argument, "correct horse") {
			t.Fatalf("the passphrase appeared in an argument")
		}
	}
	if string(command.Stdin) != "correct horse" {
		t.Fatalf("Stdin = %q, want the passphrase", command.Stdin)
	}
	if strings.Contains(strings.Join(command.Env, "\n"), "correct horse") {
		t.Fatalf("the passphrase appeared in the environment")
	}
	assertScrubbedEnvironment(t, command)
}

func TestKeyAgentNeverGivesAChildAnAskpassEnvironment(t *testing.T) {
	recorder := &recordingRunner{outputs: []platform.Output{{}, {}, {}}}
	agent := process.NewKeyAgent(recorder, installedToolchain(), agentLookup)

	if err := agent.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: "/Users/example/.ssh/id_work",
		Passphrase:     []byte("correct horse"),
	}); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if _, err := agent.List(context.Background()); err != nil {
		t.Fatalf("List error = %v", err)
	}
	if err := agent.Remove(context.Background(), "/Users/example/.ssh/id_work.pub"); err != nil {
		t.Fatalf("Remove error = %v", err)
	}

	if len(recorder.commands) != 3 {
		t.Fatalf("commands = %#v, want three", recorder.commands)
	}
	for index, command := range recorder.commands {
		t.Run(strings.Join(command.Arguments, " "), func(t *testing.T) {
			if command.Path != "/usr/bin/ssh-add" {
				t.Errorf("commands[%d].Path = %q", index, command.Path)
			}
			assertScrubbedEnvironment(t, command)
		})
	}
}

func TestKeyAgentReportsRejectionWithoutLeakingTheHomePath(t *testing.T) {
	recorder := &recordingRunner{outputs: []platform.Output{{
		ExitCode: 1,
		Stderr:   []byte("Bad passphrase, try again for /Users/example/.ssh/id_work: \n"),
	}}}
	agent := process.NewKeyAgent(recorder, installedToolchain(), agentLookup)

	err := agent.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: "/Users/example/.ssh/id_work",
		Passphrase:     []byte("wrong"),
	})
	if !errors.Is(err, platform.ErrAgentRejected) {
		t.Fatalf("error = %v, want ErrAgentRejected", err)
	}
	if strings.Contains(err.Error(), "/Users/example") {
		t.Fatalf("the error carried the absolute home path: %v", err)
	}
	if !strings.Contains(err.Error(), "~/.ssh/id_work") {
		t.Fatalf("the error lost the useful part of the message: %v", err)
	}
}

func TestKeyAgentListParsesIdentitiesAndAnEmptyAgent(t *testing.T) {
	recorder := &recordingRunner{outputs: []platform.Output{{
		Stdout: []byte("256 SHA256:abcdef aida@laptop (ED25519)\n2048 SHA256:012345 work key (RSA)\n"),
	}}}
	identities, err := process.NewKeyAgent(recorder, installedToolchain(), agentLookup).List(context.Background())
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("identities = %#v, want two", identities)
	}
	if identities[0].Bits != 256 || identities[0].Fingerprint != "SHA256:abcdef" || identities[0].Algorithm != "ED25519" {
		t.Errorf("identities[0] = %#v", identities[0])
	}
	if identities[1].Comment != "work key" {
		t.Errorf("identities[1].Comment = %q, want %q", identities[1].Comment, "work key")
	}
	if arguments := strings.Join(recorder.commands[0].Arguments, " "); arguments != "-l -E sha256" {
		t.Errorf("Arguments = %q, want %q", arguments, "-l -E sha256")
	}

	empty := &recordingRunner{outputs: []platform.Output{{
		ExitCode: 1,
		Stdout:   []byte("The agent has no identities.\n"),
	}}}
	none, err := process.NewKeyAgent(empty, installedToolchain(), agentLookup).List(context.Background())
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("identities = %#v, want none", none)
	}
}

func TestKeyAgentRefusesWhenNoAgentSocketIsAdvertised(t *testing.T) {
	recorder := &recordingRunner{}
	agent := process.NewKeyAgent(recorder, installedToolchain(), func(string) (string, bool) { return "", false })

	if agent.Available(context.Background()) {
		t.Fatalf("Available = true without SSH_AUTH_SOCK")
	}
	if err := agent.Add(context.Background(), platform.AgentAddRequest{PrivateKeyPath: "/Users/example/.ssh/id_work"}); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("Add error = %v, want ErrAgentUnavailable", err)
	}
	if _, err := agent.List(context.Background()); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("List error = %v, want ErrAgentUnavailable", err)
	}
	if err := agent.Remove(context.Background(), "/Users/example/.ssh/id_work.pub"); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("Remove error = %v, want ErrAgentUnavailable", err)
	}
	if len(recorder.commands) != 0 {
		t.Fatalf("a command ran without an agent: %#v", recorder.commands)
	}
}

func TestKeyAgentRefusesWhenSSHAddIsNotInstalled(t *testing.T) {
	recorder := &recordingRunner{}
	missing := toolchainStub{keyAddErr: errors.New("ssh-add not found")}
	agent := process.NewKeyAgent(recorder, missing, agentLookup)

	if agent.Available(context.Background()) {
		t.Fatalf("Available = true without an ssh-add to run")
	}
	if err := agent.Add(context.Background(), platform.AgentAddRequest{PrivateKeyPath: "/Users/example/.ssh/id_work"}); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("Add error = %v, want ErrAgentUnavailable", err)
	}
	if len(recorder.commands) != 0 {
		t.Fatalf("a command ran without ssh-add: %#v", recorder.commands)
	}
}
