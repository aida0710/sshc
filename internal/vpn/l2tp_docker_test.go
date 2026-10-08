//go:build linux

package vpn

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 本物の strongSwan のサーバーを立て、L2TP/IPsec の IPsec の段（IKE と transport mode の
// SA）を、製品のイメージと手順（backend-l2tp_ipsec.sh の start_ipsec）で確かめる。
//
// IKEv1 では、proposal ごとに暗号・MAC・DH群の先頭の1つしか送られない。どの組み合わせを
// 並べたかが、そのまま接続できる装置を決める。イメージのプラグインが変わると strongSwan の
// 既定の候補も変わるので、ここで実際に交渉させる。
//
// L2TP と PPP のサーバーは立てない。SSHC_VPN_DOCKER_TEST=1 のときだけ走る。

const (
	// l2tpTestPreSharedKey は、テスト用のサーバーとの事前共有鍵である。
	l2tpTestPreSharedKey = "fixture-psk"
	// l2tpTestServerStartTimeout は、サーバーが接続を受け付けるまで待つ上限である。
	l2tpTestServerStartTimeout = 30 * time.Second
	// l2tpTestDeadlineSeconds は、IPsec を確立するまでの締め切りである。同じ Docker の
	// 回線の上では1秒もかからない。
	l2tpTestDeadlineSeconds = 30
	// l2tpUnansweredDeadlineSeconds は、応答しない相手へ IKE を送り続ける締め切りである。
	// charon は4秒後に1回目の再送をするので、それを含む長さにする。
	l2tpUnansweredDeadlineSeconds = 8
)

// startL2TPIPsecServer は、ike の組み合わせだけを受け付ける IKEv1 のサーバーを立て、
// Docker の通常の回線で届くアドレスを返す。
func startL2TPIPsecServer(t *testing.T, manager *Manager, ctx context.Context, image, ike string) string {
	t.Helper()
	name := fmt.Sprintf("sshc-vpn-test-l2tp-%d-%d", os.Getpid(), time.Now().UnixNano())
	configuration := strings.Join([]string{
		"conn l2tp",
		"    keyexchange=ikev1",
		"    authby=secret",
		"    type=transport",
		"    left=%any",
		"    leftprotoport=17/1701",
		"    right=%any",
		"    rightprotoport=17/1701",
		// 末尾の ! で、指定した組み合わせのほかは受け付けない。
		"    ike=" + ike + "!",
		"    esp=aes256-sha1!",
		"    auto=add",
	}, "\n")
	script := strings.Join([]string{
		"set -eu",
		"cat >/etc/ipsec.conf <<'EOF'",
		configuration,
		"EOF",
		`printf ': PSK "%s"\n' '` + l2tpTestPreSharedKey + `' >/etc/ipsec.secrets`,
		"chmod 600 /etc/ipsec.secrets",
		// 失敗したときに、選んだ組み合わせと断った理由を docker logs で見られるようにする。
		"printf 'charon {\\n  filelog {\\n    stderr {\\n      default = 1\\n      cfg = 2\\n    }\\n  }\\n}\\n'" +
			" >/etc/strongswan.d/zz-test.conf",
		"exec ipsec start --nofork",
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("L2TP/IPsec のサーバーを起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.combined(context.Background(), "logs", "--tail", "60", name)
			t.Logf("L2TP/IPsec のサーバーのログ:\n%s", logs)
		}
		_, _ = manager.docker.output(context.Background(), "rm", "--force", name)
	})
	// starter が接続を charon へ渡し終えるまで待つ。渡す前に届いた IKE は断られる。
	deadline := time.Now().Add(l2tpTestServerStartTimeout)
	for {
		if _, err := manager.docker.output(ctx, "exec", name, "sh", "-c",
			"ipsec statusall 2>/dev/null | grep -q ' l2tp: '"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("L2TP/IPsec のサーバーが受け付けを始めない")
		}
		time.Sleep(time.Second)
	}
	address, err := manager.docker.output(ctx, "container", "inspect", "--format",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(address)
}

// startL2TPIPsec は、製品のイメージの中で、agent と同じ手順で server との IPsec を
// 確立し、コンテナの出力（失敗したときは、agent が見せるログを含む）を返す。
func startL2TPIPsec(t *testing.T, manager *Manager, ctx context.Context, image, server string, deadlineSeconds int) (string, error) {
	t.Helper()
	profile := Profile{
		Name:    "l2tp-test",
		Backend: L2TPIPsec,
		L2TP:    &L2TPSettings{Server: server, Username: "fixture"},
	}
	secrets := Secrets{L2TP: &L2TPSecrets{Password: "fixture-password", PreSharedKey: l2tpTestPreSharedKey}}
	document, err := newAgentDocument(profile, secrets, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// agent.sh のうち、IPsec の段が使う関数だけを持ち込む。L2TP の段とトンネルの見張りは
	// 走らせない。
	script := agentFunctions(t, "pause", "fail", "seconds_since_boot", "remaining_seconds",
		"timeout_seconds", "wait_for_step", "resolve_first_ipv4", "start_shutdown_budget",
		"shutdown_seconds_left", "shutdown_timeout_seconds") + `
set -eu
runtime=` + agentRuntimeDirectory + `
shared_directory=$runtime
backend_directory=/usr/local/lib/sshc-vpn
profile="$runtime/profile.json"
shutdown_seconds=5
cat >"$profile"
deadline=$(($(seconds_since_boot) + $1))
. "$backend_directory/backend-l2tp_ipsec.sh"
backend_read
resolve_server_address "$runtime/ipsec.conf"
start_ipsec
cat "$runtime/ipsec.log"
`
	return manager.docker.run(ctx, dockerCall{
		arguments: []string{"run", "--rm", "--interactive", "--network", "bridge", "--cap-add", "NET_ADMIN",
			"--tmpfs", agentRuntimeDirectory + ":rw,mode=700", "--entrypoint", "sh", image,
			"-c", script, "l2tp-test", strconv.Itoa(deadlineSeconds)},
		input:              document,
		mergeOutput:        true,
		callerShowsFailure: true,
	})
}

// 既定の暗号スイートで、1つの組み合わせしか受け付けないサーバーとも IPsec を確立する。
// MODP と SHA-1 だけを受け付けるサーバーは、strongSwan の既定の候補から送られる1組
// （AES-128・SHA-256・ECP-256）には応答しない。
func TestTheDefaultL2TPProposalsEstablishIPsecWithServersThatAcceptOneSuite(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	for _, ike := range []string{
		"aes256-sha1-modp2048",
		"aes128-sha1-modp1024",
		"aes128-sha256-modp3072",
		// ipsec.conf が後ろに足す strongSwan の既定の候補の組み合わせ。
		"aes128-sha256-ecp256",
	} {
		t.Run(ike, func(t *testing.T) {
			server := startL2TPIPsecServer(t, manager, ctx, image, ike)
			output, err := startL2TPIPsec(t, manager, ctx, image, server, l2tpTestDeadlineSeconds)
			if err != nil {
				t.Fatalf("IPsec を確立できない: %v", err)
			}
			// charon のログは、agent が失敗したときに見せる ipsec.log に入っている。
			for _, want := range []string{"IPsecの接続が完了しました。", "IKE_SA " + connectionName + "[1] established"} {
				if !strings.Contains(output, want) {
					t.Errorf("出力に %q が無い:\n%s", want, output)
				}
			}
		})
	}
}

// 応答しないサーバーへの IKE の再送は、締め切りで止めても、失敗のログに残る。
func TestAnUnansweredL2TPNegotiationLeavesTheRetransmitsInTheFailureLog(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	// IKE を受け付けない相手。届いたパケットには誰も答えない。
	name := fmt.Sprintf("sshc-vpn-test-l2tp-silent-%d", os.Getpid())
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name, "--network", "bridge",
		"--entrypoint", "sleep", image, "120"); err != nil {
		t.Fatalf("応答しない相手を起動できない: %v", err)
	}
	t.Cleanup(func() { _, _ = manager.docker.output(context.Background(), "rm", "--force", name) })
	address, err := manager.docker.output(ctx, "container", "inspect", "--format",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatal(err)
	}

	_, err = startL2TPIPsec(t, manager, ctx, image, strings.TrimSpace(address), l2tpUnansweredDeadlineSeconds)
	if err == nil {
		t.Fatal("応答しない相手と IPsec を確立したことになった")
	}
	for _, want := range []string{
		"IPsecのネゴシエーションに失敗しました。",
		"[ipsec] ",
		"sending retransmit 1 of request message ID 0",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("失敗の出力に %q が無い:\n%v", want, err)
		}
	}
}
