package diagnostics_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/diagnostics"
	"sshc/internal/storage"
)

const serviceConfig = "Host bastion\n" +
	"\tHostName 203.0.113.10\n" +
	"\tUser ops\n" +
	"\tPort 2222\n" +
	"\n" +
	"Host risky\n" +
	"\tProxyCommand /usr/bin/nc %h %p\n"

func newServiceWorkspace(t *testing.T, contents string) *storage.Workspace {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func newTestService(t *testing.T, probe *scriptedProbe) *diagnostics.Service {
	t.Helper()
	service := diagnostics.NewService(newServiceWorkspace(t, serviceConfig), probe.dial)
	service.Reachability = diagnostics.Reachability{
		Dialer: dialerFunc(func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Err: errRefusedForTest}
		}),
	}
	return service
}

var errRefusedForTest = net.UnknownNetworkError("refused in test")

type countingFileSystem struct {
	storage.OSFileSystem
	reads int
}

func (fileSystem *countingFileSystem) ReadFile(path string) ([]byte, error) {
	fileSystem.reads++
	return fileSystem.OSFileSystem.ReadFile(path)
}

func TestConnectionSnapshotDerivesEverythingFromOneGraphRead(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(serviceConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	fileSystem := &countingFileSystem{}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	service := diagnostics.NewService(workspace, (&scriptedProbe{}).dial)

	snapshot, err := service.ConnectionSnapshot("bastion")
	if err != nil {
		t.Fatalf("ConnectionSnapshot = %v", err)
	}
	if fileSystem.reads != 1 {
		t.Fatalf("configuration reads = %d, want 1", fileSystem.reads)
	}
	if snapshot.Hostname != "203.0.113.10" || snapshot.Port != "2222" || snapshot.User != "ops" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if string(snapshot.Config) != serviceConfig {
		t.Fatalf("config snapshot = %q", snapshot.Config)
	}
	if len(snapshot.Report.Directives) != 1 {
		t.Fatalf("report = %#v", snapshot.Report)
	}
}

func TestServiceDestinationUsesTheEngineSoABlockedEvaluationStillWorks(t *testing.T) {
	probe := &scriptedProbe{}
	service := newTestService(t, probe)

	hostname, port, err := service.Destination("bastion")
	if err != nil {
		t.Fatalf("Destination = %v", err)
	}
	if hostname != "203.0.113.10" || port != "2222" {
		t.Fatalf("destination = %s:%s", hostname, port)
	}

	unknownHostname, unknownPort, err := service.Destination("unlisted")
	if err != nil {
		t.Fatalf("Destination = %v", err)
	}
	if unknownHostname != "unlisted" || unknownPort != "22" {
		t.Errorf("defaults = %s:%s, want unlisted:22", unknownHostname, unknownPort)
	}

	result, err := service.Reach(context.Background(), "bastion")
	if err != nil {
		t.Fatalf("Reach = %v", err)
	}
	if result.Address != "203.0.113.10:2222" || result.Notice == "" {
		t.Errorf("result = %#v", result)
	}
	if len(probe.calls) != 0 {
		t.Fatal("reachability must not authenticate")
	}
}

func TestServiceConfigCheckSummarisesTheIncludeGraph(t *testing.T) {
	report, err := newTestService(t, &scriptedProbe{}).ConfigCheck()
	if err != nil {
		t.Fatalf("ConfigCheck = %v", err)
	}
	if len(report.Files) != 1 || !report.Files[0].Editable || report.Files[0].Missing {
		t.Fatalf("files = %#v", report.Files)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Severity > 0 {
			t.Errorf("unexpected diagnostic: %#v", diagnostic)
		}
	}
}

// TestServiceAuthenticateSanitisesTheHomePathOutOfReportedOutput は、このプロセス
// から出ていくものを守る。失敗の説明には鍵のパスが入りうるので、そこに書かれた
// ホームディレクトリを ~ へ置き換えてから返す。
func TestServiceAuthenticateSanitisesTheHomePathOutOfReportedOutput(t *testing.T) {
	probe := &scriptedProbe{}
	service := newTestService(t, probe)
	home := service.Workspace.Home()
	probe.err = errors.New("ssh: unable to authenticate; " + home + "/.ssh/id_ed25519 was refused")

	result, err := service.Authenticate(context.Background(), "bastion", true)
	if err != nil {
		t.Fatalf("Authenticate = %v", err)
	}
	if strings.Contains(result.Detail, home) {
		t.Fatalf("the reported detail names the home directory: %q", result.Detail)
	}
	if !strings.Contains(result.Detail, "~/.ssh/id_ed25519") {
		t.Errorf("detail = %q, want the path rewritten to ~", result.Detail)
	}
	if !strings.Contains(result.Detail, "unable to authenticate") {
		t.Error("sanitising removed the reason for the failure")
	}
}

func TestServiceProjectedValueReadsTheEngineWithoutRunningSSH(t *testing.T) {
	probe := &scriptedProbe{}
	service := newTestService(t, probe)

	user, ok := service.ProjectedValue("bastion", "user")
	if !ok || user != "ops" {
		t.Fatalf("ProjectedValue = %q, %v", user, ok)
	}
	if _, ok := service.ProjectedValue("bastion", "identityfile"); ok {
		t.Error("a keyword the configuration does not set must report false")
	}
	if _, ok := service.ProjectedValue("bad alias", "user"); ok {
		t.Error("an unsafe alias must not be projected")
	}
	if len(probe.calls) != 0 {
		t.Fatal("projecting a value reached the network")
	}
}
