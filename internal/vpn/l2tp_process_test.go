package vpn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestL2TPFailuresPreserveDaemonLogsAndOnlyBlameExplicitAuthenticationRejection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container backend requires a POSIX shell")
	}
	for _, scenario := range []struct {
		name      string
		pppLog    string
		reason    string
		connected bool
	}{
		{name: "no PPP process", reason: "timeout"},
		{name: "PPP device unavailable", pppLog: "Couldn't open the /dev/ppp device: Operation not permitted", reason: "timeout"},
		{name: "IPCP timed out after authentication", pppLog: "CHAP authentication succeeded\nIPCP: timeout sending Config-Requests", reason: "timeout"},
		{name: "server rejects CHAP", pppLog: "CHAP authentication failed", reason: "ppp_authentication"},
		{name: "server rejects PAP", pppLog: "PAP authentication failed", reason: "ppp_authentication"},
		{name: "address assigned", connected: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			stageBackendScript(t, directory, L2TPIPsec)
			for name, contents := range map[string]string{
				"ipsec.log":  "connection established",
				"xl2tpd.log": "Connecting to host", "ppp.log": scenario.pppLog,
			} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// No network commands run: only the production wait/classification logic.
			script := `runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
l2tp_pid=$$
connected=$2
backend_alive() { [ "$connected" = connected ]; }
remaining_seconds() { echo 0; }
fail() { printf 'reason=%s\n' "$1"; exit 1; }
wait_for_ppp_address
`
			state := "waiting"
			if scenario.connected {
				state = "connected"
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			output, runErr := exec.CommandContext(ctx, "sh", "-c", script, "l2tp-test", directory, state).CombinedOutput()
			if scenario.connected {
				if runErr != nil || len(output) != 0 {
					t.Fatalf("connected tunnel failed: %v: %s", runErr, output)
				}
				return
			}
			if runErr == nil || !strings.Contains(string(output), "reason="+scenario.reason) {
				t.Fatalf("got %v: %s; want %s", runErr, output, scenario.reason)
			}
			for _, evidence := range []string{"[ipsec] connection established", "[xl2tpd] Connecting to host", "[ppp]"} {
				if !strings.Contains(string(output), evidence) {
					t.Errorf("missing daemon log %q: %s", evidence, output)
				}
			}
			if !strings.HasSuffix(string(output), "reason="+scenario.reason+"\n") {
				t.Errorf("failure published before logs were collected: %s", output)
			}
		})
	}
}

func TestL2TPStartsOnlyAfterItsIPsecTransportSAIsInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container backend requires a POSIX shell")
	}
	for _, scenario := range []struct {
		name      string
		status    string
		installed bool
	}{
		{name: "IKE established but Quick Mode rejected", status: "sshc-vpn[1]: ESTABLISHED 1 second ago"},
		{name: "another connection installed", status: "other-vpn{1}: INSTALLED, TRANSPORT, reqid 1"},
		{name: "tunnel mode installed", status: "sshc-vpn{1}: INSTALLED, TUNNEL, reqid 1"},
		{name: "transport SA installed", status: "    sshc-vpn{2}:  INSTALLED, TRANSPORT, reqid 1", installed: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			stageBackendScript(t, directory, L2TPIPsec)
			// Match stroke's successful exit code even when no CHILD_SA was created.
			script := agentFunctions(t, "wait_for_step") + `
runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
status=$2
timeout_seconds() { echo 1; }
timeout() { shift; "$@"; }
ipsec() {
    if [ "$1" = status ]; then printf '%s\n' "$status"; fi
    return 0
}
fail() { printf 'reason=%s\n' "$1"; exit 1; }
establish_ipsec
echo l2tp-start
`
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			output, runErr := exec.CommandContext(ctx, "sh", "-c", script, "ipsec-test", directory, scenario.status).CombinedOutput()
			if scenario.installed {
				if runErr != nil || string(output) != "l2tp-start\n" {
					t.Fatalf("installed transport was rejected: %v: %s", runErr, output)
				}
				return
			}
			if runErr == nil || strings.Contains(string(output), "l2tp-start") || !strings.Contains(string(output), "reason=ipsec_negotiation") {
				t.Fatalf("unprotected transport was accepted: %v: %s", runErr, output)
			}
			if !strings.Contains(string(output), scenario.status) {
				t.Fatalf("failure omitted IPsec state: %s", output)
			}
		})
	}
}

// L2TP の切断から IPsec の後始末までは、どの段が応えなくても、合わせて止める手順の
// 持ち時間に収まる。docker stop の猶予を超えると、SIGKILL で途中で終わる。
func TestL2TPShutdownFitsTheShutdownBudget(t *testing.T) {
	if _, err := os.Stat("/proc/uptime"); err != nil {
		t.Skip("/proc/uptime が無い（コンテナの中は Linux）")
	}
	const shutdownSeconds = 3
	const slack = 700 * time.Millisecond
	directory := t.TempDir()
	stageBackendScript(t, directory, L2TPIPsec)
	if err := os.WriteFile(filepath.Join(directory, "l2tp-control"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// どのコマンドも応えず、timeout の上限まで止まる。PPP のアドレスも消えない。
	script := agentFunctions(t, "seconds_since_boot", "start_shutdown_budget", "shutdown_seconds_left",
		"shutdown_timeout_seconds") + `
runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
shutdown_seconds=` + strconv.Itoa(shutdownSeconds) + `
timeout() { sleep "$1"; }
backend_alive() { return 0; }
start_shutdown_budget
backend_down
echo down
`
	started := time.Now()
	output := runBackendScript(t, script, directory)

	if took := time.Since(started); took > shutdownSeconds*time.Second+slack {
		t.Fatalf("止める手順に %v かかった（持ち時間 %d 秒）: %s", took, shutdownSeconds, output)
	}
	if output != "down\n" {
		t.Fatalf("output = %q", output)
	}
}
