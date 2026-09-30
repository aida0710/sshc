//go:build linux

package vpn

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// 本物の strongSwan のサーバーを立て、IKEv2 の経路が EAP と事前共有鍵のそれぞれで
// 接続先まで届くこと、断られた理由を区別することを確かめる。
//
// サーバーは製品のイメージ（charon と swanctl が入っている）で動かす。証明書は
// テストの中で作る。SSHC_VPN_DOCKER_TEST=1 のときだけ走る。

const (
	// ikev2ServerName は、テスト用のサーバーの証明書の名前であり、サーバーの ID である。
	ikev2ServerName = "vpn.test"
	// ikev2TunnelTarget は、テスト用のサーバーがトンネルの中でだけ待ち受けるアドレスで
	// ある。Docker の通常の回線からは届かないので、返事が来ればトンネルを通っている。
	ikev2TunnelTarget = "10.66.0.1"
	// ikev2PushedDNS は、サーバーが配る DNS サーバーである。コンテナは使わない。
	ikev2PushedDNS = "10.66.0.53"
	// テスト用のサーバーが受け付ける利用者である。
	ikev2Username     = "fixture"
	ikev2Password     = "fixture-password"
	ikev2PSKIdentity  = "psk-client"
	ikev2PreSharedKey = "fixture-psk"
	// ikev2ServerStartTimeout は、サーバーが受け付けを始めるまで待つ上限である。
	ikev2ServerStartTimeout = time.Minute
	// ikev2TunnelLostTimeout は、サーバーが SA を消してから、経路が止まるまで待つ上限
	// である。agent がトンネルを見に行く間隔（5秒）に余裕を足す。
	ikev2TunnelLostTimeout = 30 * time.Second
)

// ikev2ServerConfiguration は、EAP-MSCHAPv2 と事前共有鍵の両方を受け付けるサーバーの
// swanctl.conf である。サーバーはトラフィックセレクターに 0.0.0.0/0 を配り、DNS も
// 配る。どちらもコンテナの経路表と resolv.conf に入らないことを確かめる。
var ikev2ServerConfiguration = strings.Join([]string{
	"connections {",
	"    eap {",
	"        version = 2",
	"        pools = clients",
	"        local {",
	"            auth = pubkey",
	"            certs = server.pem",
	"            id = " + ikev2ServerName,
	"        }",
	"        remote {",
	"            auth = eap-mschapv2",
	"            eap_id = %any",
	"        }",
	"        children {",
	"            tunnel {",
	"                local_ts = 0.0.0.0/0",
	"            }",
	"        }",
	"    }",
	"    psk {",
	"        version = 2",
	"        pools = clients",
	"        local {",
	"            auth = psk",
	"            id = " + ikev2ServerName,
	"        }",
	"        remote {",
	"            auth = psk",
	"            id = " + ikev2PSKIdentity,
	"        }",
	"        children {",
	"            tunnel {",
	"                local_ts = 0.0.0.0/0",
	"            }",
	"        }",
	"    }",
	"}",
	"pools {",
	"    clients {",
	"        addrs = 10.66.1.0/24",
	"        dns = " + ikev2PushedDNS,
	"    }",
	"}",
	"secrets {",
	"    eap-fixture {",
	"        id = " + ikev2Username,
	`        secret = "` + ikev2Password + `"`,
	"    }",
	"    ike-fixture {",
	"        id-1 = " + ikev2PSKIdentity,
	"        id-2 = " + ikev2ServerName,
	`        secret = "` + ikev2PreSharedKey + `"`,
	"    }",
	"}",
	"",
}, "\n")

// ikev2Server は、テスト用に立てた IKEv2 のサーバーである。
type ikev2Server struct {
	name string
	// address は、Docker の通常の回線で届くサーバーのアドレスである。
	address string
	// authority は、サーバーの証明書を発行した CA である。
	authority testAuthority
}

// startIKEv2Server は、本物の strongSwan のサーバーを1台立てる。
func startIKEv2Server(t *testing.T, manager *Manager, ctx context.Context, image string) ikev2Server {
	t.Helper()
	authority := newTestAuthority(t, "sshc test CA")
	certificate, key := authority.issueServerCertificate(t, ikev2ServerName)
	name := fmt.Sprintf("sshc-vpn-test-ikev2-%d", os.Getpid())
	script := strings.Join([]string{
		"set -eu",
		// 本番の agent と同じく、設定が届くのを待つ。--detach で起動するので、
		// 標準入力からは読めない。
		"seconds=0",
		"while [ ! -f /run/ikev2-server/ready ]; do",
		"  if [ \"$seconds\" -ge 30 ]; then echo '設定が届かなかった' >&2; exit 1; fi",
		"  sleep 1; seconds=$((seconds + 1))",
		"done",
		"cp /run/ikev2-server/server.pem /etc/swanctl/x509/server.pem",
		"cp /run/ikev2-server/server-key.pem /etc/swanctl/private/server-key.pem",
		"cp /run/ikev2-server/swanctl.conf /etc/swanctl/swanctl.conf",
		"printf 'charon {\\n  filelog {\\n    stderr {\\n      default = 1\\n    }\\n  }\\n}\\n'" +
			" > /etc/strongswan.d/zz-test.conf",
		// トンネルの中からだけ届く相手。
		"ip address add " + ikev2TunnelTarget + "/32 dev lo",
		fmt.Sprintf("socat TCP-LISTEN:%d,bind=%s,fork,reuseaddr SYSTEM:'echo tunnelled' &", echoPort, ikev2TunnelTarget),
		"/usr/lib/ipsec/charon &",
		"seconds=0",
		"while [ ! -S /var/run/charon.vici ]; do",
		"  if [ \"$seconds\" -ge 30 ]; then echo 'charon が起動しない' >&2; exit 1; fi",
		"  sleep 1; seconds=$((seconds + 1))",
		"done",
		"swanctl --load-all",
		"echo 'ikev2 server ready'",
		"wait",
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("IKEv2 のサーバーを起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.combined(context.Background(), "logs", "--tail", "60", name)
			t.Logf("IKEv2 のサーバーのログ:\n%s", logs)
		}
		_, _ = manager.docker.output(context.Background(), "rm", "--force", name)
	})
	for file, contents := range map[string]string{
		"server.pem": certificate, "server-key.pem": key, "swanctl.conf": ikev2ServerConfiguration,
	} {
		if _, err := manager.docker.outputWithInput(ctx, contents, "exec", "-i", name,
			"sh", "-c", "mkdir -p /run/ikev2-server && cat > /run/ikev2-server/"+file); err != nil {
			t.Fatalf("サーバーへ %s を渡せない: %v", file, err)
		}
	}
	if _, err := manager.docker.output(ctx, "exec", name, "touch", "/run/ikev2-server/ready"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(ikev2ServerStartTimeout)
	for {
		logs, err := manager.docker.combined(ctx, "logs", "--tail", "20", name)
		if err == nil && strings.Contains(logs, "ikev2 server ready") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("IKEv2 のサーバーが受け付けを始めない: %v\n%s", err, logs)
		}
		time.Sleep(time.Second)
	}
	address, err := manager.docker.output(ctx, "container", "inspect", "--format",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatal(err)
	}
	return ikev2Server{name: name, address: strings.TrimSpace(address), authority: authority}
}

// eapProfile は、このサーバーへ EAP で接続するプロファイルである。サーバーは
// Docker のアドレスで指すので、証明書の名前はサーバーの ID として書く。
func (server ikev2Server) eapProfile(name string) Profile {
	return Profile{
		Name:    name,
		Backend: IKEv2,
		IKEv2: &IKEv2Settings{
			Server:         server.address,
			Authentication: IKEv2AuthenticationEAP,
			Identity:       ikev2Username,
			ServerIdentity: ikev2ServerName,
			CACertificate:  server.authority.PEM,
		},
	}
}

// pskProfile は、このサーバーへ事前共有鍵で接続するプロファイルである。
func (server ikev2Server) pskProfile(name string) Profile {
	return Profile{
		Name:    name,
		Backend: IKEv2,
		IKEv2: &IKEv2Settings{
			Server:         server.address,
			Authentication: IKEv2AuthenticationPSK,
			Identity:       ikev2PSKIdentity,
			ServerIdentity: ikev2ServerName,
		},
	}
}

// startIKEv2Test は、イメージとテスト用のサーバーを用意する。
func startIKEv2Test(t *testing.T) (*Manager, context.Context, ikev2Server) {
	t.Helper()
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	return manager, ctx, startIKEv2Server(t, manager, ctx, image)
}

// EAP（ユーザー名とパスワード）で接続し、トンネルの中にいる相手へ届く。サーバーが
// 配る経路と DNS はコンテナに入らない。
func TestAConnectionReachesTheTargetThroughAnIKEv2EAPTunnel(t *testing.T) {
	manager, ctx, server := startIKEv2Test(t)
	profile := server.eapProfile("ikev2-eap")
	secrets := Secrets{IKEv2: &IKEv2Secrets{Password: ikev2Password}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	requireAnswer(t, manager, ctx, dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", ikev2TunnelTarget, echoPort)}, "tunnelled")

	status, err := manager.Status(ctx, profile.Name)
	if err != nil || status.Tunnel.Interface != ikev2Link.Name || !strings.HasPrefix(status.Tunnel.Address, "10.66.1.") {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	client := manager.containerName(profile.Name)
	// サーバーは 0.0.0.0/0 を配ったが、接続先以外はトンネルへ向かわない。
	route, err := manager.docker.output(ctx, "exec", client, "ip", "route", "get", "192.0.2.10")
	if err != nil || strings.Contains(route, ikev2Link.Name) {
		t.Fatalf("接続先でないアドレスがトンネルへ向かう: %q, %v", route, err)
	}
	resolver, err := manager.docker.output(ctx, "exec", client, "cat", "/etc/resolv.conf")
	if err != nil || strings.Contains(resolver, ikev2PushedDNS) {
		t.Fatalf("サーバーが配った DNS が resolv.conf に入った: %q, %v", resolver, err)
	}

	// 停止するときはサーバーへ IKE_SA の削除を伝える。サーバーの側に SA が残らない。
	if err := manager.Stop(ctx, profile.Name); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	sas, err := manager.docker.output(ctx, "exec", server.name, "swanctl", "--list-sas")
	if err != nil || strings.Contains(sas, "ESTABLISHED") {
		t.Fatalf("停止したあとも、サーバーに SA が残った: %q, %v", sas, err)
	}
}

// 事前共有鍵と ID で接続し、トンネルの中にいる相手へ届く。サーバーが SA を消すと、
// 経路も止まる。
func TestAConnectionReachesTheTargetThroughAnIKEv2PSKTunnel(t *testing.T) {
	manager, ctx, server := startIKEv2Test(t)
	profile := server.pskProfile("ikev2-psk")
	secrets := Secrets{IKEv2: &IKEv2Secrets{PreSharedKey: ikev2PreSharedKey}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	requireAnswer(t, manager, ctx, dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", ikev2TunnelTarget, echoPort)}, "tunnelled")

	if _, err := manager.docker.output(ctx, "exec", server.name, "swanctl", "--terminate", "--ike", "psk"); err != nil {
		t.Fatalf("サーバーの SA を消せない: %v", err)
	}
	deadline := time.Now().Add(ikev2TunnelLostTimeout)
	for {
		status, err := manager.Status(ctx, profile.Name)
		if err != nil {
			t.Fatalf("Status = %v", err)
		}
		if !status.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("トンネルが無くなったのに、経路が残った")
		}
		time.Sleep(time.Second)
	}
}

// 接続できなかった理由を、認証、サーバーの検証、暗号スイートで分けて返す。どの場合も、
// 見せる文面にシークレットは現れない。
func TestIKEv2RefusalsSayWhy(t *testing.T) {
	manager, ctx, server := startIKEv2Test(t)
	for _, test := range []struct {
		name    string
		profile Profile
		secrets Secrets
		want    FailureReason
		// logs は、コンテナのログに現れるはずの行である。空なら確かめない。
		logs *regexp.Regexp
	}{
		{"パスワードが違う", server.eapProfile("ikev2-bad-password"),
			Secrets{IKEv2: &IKEv2Secrets{Password: "wrong-password"}}, FailureIKEAuthentication, nil},
		{"事前共有鍵が違う", server.pskProfile("ikev2-bad-psk"),
			Secrets{IKEv2: &IKEv2Secrets{PreSharedKey: "wrong-psk"}}, FailureIKEAuthentication, nil},
		{"CA を指定しなければ公的な認証局だけを信頼する", func() Profile {
			profile := server.eapProfile("ikev2-public-ca")
			profile.IKEv2.CACertificate = ""
			return profile
		}(), Secrets{IKEv2: &IKEv2Secrets{Password: ikev2Password}}, FailureIKEServerUnverified,
			// 公的な認証局の証明書は、実際に読み込まれている。
			regexp.MustCompile(`公的な認証局の証明書を[1-9][0-9]*件読み込みました。`)},
		{"別の CA の証明書", func() Profile {
			profile := server.eapProfile("ikev2-other-ca")
			profile.IKEv2.CACertificate = newTestAuthority(t, "other CA").PEM
			return profile
		}(), Secrets{IKEv2: &IKEv2Secrets{Password: ikev2Password}}, FailureIKEServerUnverified, nil},
		{"IKE の暗号スイートが合わない", func() Profile {
			profile := server.eapProfile("ikev2-proposal")
			profile.IKEv2.IKE = "aes128-sha1-modp1024!"
			return profile
		}(), Secrets{IKEv2: &IKEv2Secrets{Password: ikev2Password}}, FailureIKEProposalMismatch, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(func() { _ = manager.Stop(context.Background(), test.profile.Name) })

			err := manager.Start(ctx, test.profile.Name, fixedRoute(test.profile, test.secrets))

			requireFailureReason(t, err, test.want)
			logs := requireLogs(t, manager, ctx, test.profile.Name, test.secrets)
			for _, secret := range []string{test.secrets.IKEv2.Password, test.secrets.IKEv2.PreSharedKey} {
				if secret != "" && strings.Contains(err.Error()+logs, secret) {
					t.Fatalf("見せる文面にシークレットが現れた: %v / %s", err, logs)
				}
			}
			if test.logs != nil && !test.logs.MatchString(logs) {
				t.Fatalf("ログに %q が無い: %s", test.logs, logs)
			}
			// 証明書を検証できないサーバーには、パスワードを送らない。
			if test.want == FailureIKEServerUnverified && strings.Contains(logs, "EAP_MSCHAPV2") {
				t.Fatalf("検証できないサーバーと EAP を始めた: %s", logs)
			}
		})
	}
}

// 応えないサーバーを待ち続けず、応答が無いことを理由として返す。
func TestAnIKEv2ServerThatDoesNotAnswerSaysSo(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	profile := Profile{
		Name:    "ikev2-silent",
		Backend: IKEv2,
		IKEv2: &IKEv2Settings{
			// TEST-NET-1。誰も応答しない。
			Server:         "192.0.2.1",
			Authentication: IKEv2AuthenticationPSK,
			Identity:       ikev2PSKIdentity,
		},
	}
	secrets := Secrets{IKEv2: &IKEv2Secrets{PreSharedKey: ikev2PreSharedKey}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	err := manager.Start(ctx, profile.Name, fixedRoute(profile, secrets))

	requireFailureReason(t, err, FailureIKENoResponse)
}
