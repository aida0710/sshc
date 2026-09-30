package vpn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 接続できなかった理由は、charon のログから選ぶ。ログの行は、strongSwan 5.9.13 の
// サーバーに対して実際に失敗させたときのものである。
func TestIKEv2FailuresAreReadFromTheCharonLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container backend requires a POSIX shell")
	}
	for _, scenario := range []struct {
		name   string
		log    string
		reason FailureReason
	}{
		{"EAP のパスワードが違う", "[IKE] EAP-MS-CHAPv2 failed with error ERROR_AUTHENTICATION_FAILURE: '(null)'\n" +
			"[IKE] EAP_MSCHAPV2 method failed", FailureIKEAuthentication},
		{"事前共有鍵か ID が違う", "[ENC] parsed IKE_AUTH response 1 [ N(AUTH_FAILED) ]\n" +
			"[IKE] received AUTHENTICATION_FAILED notify error", FailureIKEAuthentication},
		{"サーバーの証明書を信頼できない", "[CFG] no issuer certificate found for \"CN=vpn.test\"\n" +
			"[IKE] no trusted ECDSA public key found for 'vpn.test'", FailureIKEServerUnverified},
		{"サーバーの ID が証明書と違う", "[CFG] constraint check failed: identity 'other.test' required",
			FailureIKEServerUnverified},
		{"IKE の暗号スイートが合わない", "[IKE] received NO_PROPOSAL_CHOSEN notify error", FailureIKEProposalMismatch},
		{"ESP の暗号スイートが合わない", "[IKE] received NO_PROPOSAL_CHOSEN notify, no CHILD_SA built\n" +
			"[IKE] failed to establish CHILD_SA, keeping IKE_SA", FailureIKEProposalMismatch},
		{"サーバーが応答しない", "[IKE] retransmit 1 of request with message ID 0\n" +
			"[IKE] retransmit 2 of request with message ID 0", FailureIKENoResponse},
		{"再送のあとで認証に失敗した", "[IKE] retransmit 1 of request with message ID 1\n" +
			"[IKE] received AUTHENTICATION_FAILED notify error", FailureIKEAuthentication},
		{"理由が読めない", "[IKE] received TS_UNACCEPTABLE notify, no CHILD_SA built", FailureIPsecNegotiation},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			stageBackendScript(t, directory, IKEv2)
			if err := os.WriteFile(filepath.Join(directory, "charon.log"), []byte(scenario.log+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
ikev2_failure_reason
`
			output := runBackendScript(t, script, directory)
			if strings.TrimSpace(output) != string(scenario.reason) {
				t.Fatalf("reason = %q, want %s", output, scenario.reason)
			}
		})
	}
}

// swanctl が成功を返しても、この接続の CHILD_SA が入るまでは、経路を使えることに
// しない。IKE_SA だけではデータを運べない。
func TestIKEv2IsUpOnlyAfterItsChildSAIsInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container backend requires a POSIX shell")
	}
	for _, scenario := range []struct {
		name      string
		sas       string
		installed bool
	}{
		{"IKE_SA だけ", "sshc-vpn: #1, ESTABLISHED, IKEv2, 6a97_i* 28dd_r", false},
		{"ほかの接続の CHILD_SA", "other: #1, ESTABLISHED, IKEv2\n  other: #1, reqid 1, INSTALLED, TUNNEL, ESP:AES_GCM_16-128", false},
		{"CHILD_SA が入った", "sshc-vpn: #1, ESTABLISHED, IKEv2, 7a47_i* 1388_r\n" +
			"  sshc-vpn: #1, reqid 1, INSTALLED, TUNNEL-in-UDP, ESP:AES_GCM_16-128", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			stageBackendScript(t, directory, IKEv2)
			// swanctl.conf は、swanctl が証明書を探すディレクトリに置いてある。
			configuration := filepath.Join(directory, "swanctl", "swanctl.conf")
			if err := os.MkdirAll(filepath.Dir(configuration), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configuration, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			// swanctl は、読み込みと開始には成功を返し、SA の一覧には scenario を返す。
			script := agentFunctions(t, "wait_for_step") + `
runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
sas=$2
timeout_seconds() { echo 1; }
swanctl() {
    if [ "$1" = --list-sas ]; then printf '%s\n' "$sas"; fi
    return 0
}
fail() { printf 'reason=%s\n' "$1"; exit 1; }
establish_ikev2
echo established
`
			output := runBackendScript(t, script, directory, scenario.sas)
			if scenario.installed != strings.HasSuffix(output, "established\n") {
				t.Fatalf("installed = %v, output = %s", scenario.installed, output)
			}
			if !scenario.installed && !strings.Contains(output, "reason="+string(FailureIPsecNegotiation)) {
				t.Fatalf("CHILD_SA が無いことを理由にしていない: %s", output)
			}
			if _, err := os.Stat(configuration); !os.IsNotExist(err) {
				t.Fatalf("読み込んだあとも swanctl.conf（シークレット）が残った: %v", err)
			}
		})
	}
}

// runBackendScript は、backend の手順を読み込む script を、directory を runtime にして
// 走らせ、出力を返す。手順が失敗で終わっても、出力は返す。
func runBackendScript(t *testing.T, script, directory string, arguments ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", append([]string{"-c", script, "backend-test", directory}, arguments...)...)
	output, _ := command.CombinedOutput()
	return string(output)
}
