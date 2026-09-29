//go:build linux

package vpn

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

// 本物の OpenVPN のサーバーに対して、設定ファイルで経路が成立することを確かめる。
//
// サーバーは sshc と同じイメージで起動する（イメージに openvpn が入っている）。証明書と
// 鍵はテストの中で作り、設定ファイルにインラインで埋め込む。SSHC_VPN_DOCKER_TEST=1 の
// ときだけ走る（session_docker_test.go と同じ）。

const (
	// openVPNServerTunnelAddress は、テスト用のサーバーがトンネル側で名乗るアドレスである。
	// server 10.88.0.0 255.255.255.0 の最初のアドレスになる。
	openVPNServerTunnelAddress = "10.88.0.1"
	// openVPNServerPort は、テスト用のサーバーが待ち受ける UDP のポートである。
	openVPNServerPort = 1194
	// openVPNPushedNetwork は、サーバーが経路として配るネットワークである。sshc が
	// 入れないことを確かめる。
	openVPNPushedNetwork = "10.99.0.0"
	// openVPNPushedResolver は、サーバーが DNS として配るアドレスである。sshc が使わない
	// ことを確かめる。
	openVPNPushedResolver = "10.88.0.53"
	// openVPNTestUsername と openVPNTestPassword は、サーバーが受け付ける認証情報である。
	openVPNTestUsername = "fixture"
	openVPNTestPassword = "fixture-password"
	// openVPNServerStartTimeout は、テスト用のサーバーが待ち受けを始めるまで待つ上限である。
	openVPNServerStartTimeout = 30 * time.Second
	// openVPNCertificateLifetime は、テストで作る証明書の有効期間である。
	openVPNCertificateLifetime = 24 * time.Hour
	// openVPNStaticKeyBytes は、tls-crypt の鍵の長さである（OpenVPN の Static key V1）。
	openVPNStaticKeyBytes = 256
)

// openVPNCredentials は、テストで作る証明書と鍵ひとそろいである（どれも PEM）。
type openVPNCredentials struct {
	ca, serverCertificate, serverKey, clientCertificate, clientKey, tlsCrypt string
}

// newOpenVPNCredentials は、認証局、サーバーとクライアントの証明書、tls-crypt の鍵を作る。
func newOpenVPNCredentials(t *testing.T) openVPNCredentials {
	t.Helper()
	caKey := newECDSAKey(t)
	caTemplate := certificateTemplate(t, "sshc test CA")
	caTemplate.IsCA = true
	caTemplate.BasicConstraintsValid = true
	caTemplate.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(name string, usage x509.ExtKeyUsage) (certificate, key string) {
		leafKey := newECDSAKey(t)
		template := certificateTemplate(t, name)
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement
		template.ExtKeyUsage = []x509.ExtKeyUsage{usage}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return pemBlock("CERTIFICATE", der), privateKeyPEM(t, leafKey)
	}
	credentials := openVPNCredentials{ca: pemBlock("CERTIFICATE", caDER), tlsCrypt: openVPNStaticKey(t)}
	credentials.serverCertificate, credentials.serverKey = issue("sshc-test-server", x509.ExtKeyUsageServerAuth)
	credentials.clientCertificate, credentials.clientKey = issue("sshc-test-client", x509.ExtKeyUsageClientAuth)
	return credentials
}

func newECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func certificateTemplate(t *testing.T, name string) *x509.Certificate {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		// 時計の少しのずれで、作ったばかりの証明書を「まだ有効でない」とされないようにする。
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(openVPNCertificateLifetime),
	}
}

func pemBlock(kind string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

func privateKeyPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pemBlock("PRIVATE KEY", der)
}

// openVPNStaticKey は、tls-crypt の鍵を OpenVPN の Static key V1 の形で作る（16 進で
// 1行 16 バイト）。
func openVPNStaticKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, openVPNStaticKeyBytes)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	lines := []string{"-----BEGIN OpenVPN Static key V1-----"}
	for offset := 0; offset < len(key); offset += 16 {
		lines = append(lines, hex.EncodeToString(key[offset:offset+16]))
	}
	return strings.Join(append(lines, "-----END OpenVPN Static key V1-----"), "\n") + "\n"
}

// inline は、設定ファイルのインラインのブロックを作る。
func inline(tag, body string) string {
	return "<" + tag + ">\n" + strings.TrimRight(body, "\n") + "\n</" + tag + ">\n"
}

// openVPNAuthentication は、テスト用のサーバーがクライアントを確かめる方法である。
type openVPNAuthentication int

const (
	// byClientCertificate は、クライアントの証明書で確かめる。
	byClientCertificate openVPNAuthentication = iota
	// byPassword は、ユーザー名とパスワードで確かめる。クライアントの証明書は求めない。
	byPassword
)

// openVPNServerConfig は、テスト用のサーバーの設定ファイルである。
//
// サーバーは、既定経路、経路、DNS を配る。sshc がどれも入れないことを確かめるためである。
func openVPNServerConfig(credentials openVPNCredentials, authentication openVPNAuthentication) string {
	lines := []string{
		fmt.Sprintf("port %d", openVPNServerPort),
		"proto udp",
		"dev tun",
		"topology subnet",
		"server 10.88.0.0 255.255.255.0",
		"dh none",
		"keepalive 10 60",
		`push "redirect-gateway def1"`,
		fmt.Sprintf(`push "route %s 255.255.255.0"`, openVPNPushedNetwork),
		fmt.Sprintf(`push "dhcp-option DNS %s"`, openVPNPushedResolver),
		"verb 3",
	}
	if authentication == byPassword {
		lines = append(lines,
			"verify-client-cert none",
			"username-as-common-name",
			"script-security 2",
			"auth-user-pass-verify /run/server/check-password via-file",
		)
	}
	return strings.Join(lines, "\n") + "\n" + inline("ca", credentials.ca) +
		inline("cert", credentials.serverCertificate) + inline("key", credentials.serverKey) +
		inline("tls-crypt", credentials.tlsCrypt)
}

// openVPNClientConfig は、プロバイダが配る形のクライアントの設定ファイルである。
func openVPNClientConfig(server string, credentials openVPNCredentials, authentication openVPNAuthentication) string {
	lines := []string{
		"client",
		"dev tun",
		"proto udp",
		fmt.Sprintf("remote %s %d", server, openVPNServerPort),
		"resolv-retry infinite",
		"nobind",
		"persist-key",
		"persist-tun",
		"remote-cert-tls server",
		"verb 3",
	}
	body := inline("ca", credentials.ca) + inline("tls-crypt", credentials.tlsCrypt)
	if authentication == byPassword {
		lines = append(lines, "auth-user-pass")
	} else {
		body += inline("cert", credentials.clientCertificate) + inline("key", credentials.clientKey)
	}
	return strings.Join(lines, "\n") + "\n" + body
}

// openVPNServerName は、テスト用のサーバーのコンテナ名である。
func openVPNServerName(suffix string) string {
	return fmt.Sprintf("sshc-vpn-test-openvpn-%s-%d", suffix, os.Getpid())
}

// startOpenVPNServer は、本物の OpenVPN のサーバーを1台立て、Docker の通常回線で届く
// アドレスを返す。サーバーのトンネル側（10.88.0.1）では、返事をするだけのサービスが
// 待ち受ける。トンネルの中からしか届かないので、返事が来ればトンネルを通ったことになる。
func startOpenVPNServer(
	t *testing.T, manager *Manager, ctx context.Context, image, name string, configuration string,
) string {
	t.Helper()
	script := strings.Join([]string{
		"set -eu",
		"umask 077",
		"mkdir -p /run/server",
		// 本番の agent と同じく、設定が届くのを待つ。--detach で起動するので標準入力からは
		// 読めない。
		"seconds=0",
		"while [ ! -s /run/server/server.conf ]; do",
		"  if [ \"$seconds\" -ge 30 ]; then echo '設定が届かなかった' >&2; exit 1; fi",
		"  sleep 1; seconds=$((seconds + 1))",
		"done",
		// via-file のファイルは、1行目がユーザー名、2行目がパスワードである。
		"cat > /run/server/check-password <<'EOF'",
		"#!/bin/sh",
		`[ "$(sed -n 1p "$1")" = ` + openVPNTestUsername + ` ] && [ "$(sed -n 2p "$1")" = ` + openVPNTestPassword + ` ]`,
		"EOF",
		"chmod 755 /run/server/check-password",
		fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr SYSTEM:'echo tunnelled' &", echoPort),
		"exec openvpn --config /run/server/server.conf",
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--device", "/dev/net/tun", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("OpenVPN のサーバーを起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.combined(context.Background(), "logs", "--tail", "60", name)
			t.Logf("OpenVPN のサーバーのログ:\n%s", logs)
		}
		_, _ = manager.docker.output(context.Background(), "rm", "--force", name)
	})
	if _, err := manager.docker.outputWithInput(ctx, configuration, "exec", "-i", name,
		"sh", "-c", "cat > /run/server/server.conf.pending && mv /run/server/server.conf.pending /run/server/server.conf"); err != nil {
		t.Fatalf("サーバーへ設定を渡せない: %v", err)
	}
	deadline := time.Now().Add(openVPNServerStartTimeout)
	for {
		logs, err := manager.docker.combined(ctx, "logs", "--tail", "20", name)
		if err == nil && strings.Contains(logs, "Initialization Sequence Completed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("OpenVPN のサーバーが待ち受けを始めない: %v\n%s", err, logs)
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

// openVPNRoute は、テスト用のサーバーへの経路ひとつぶんである。
type openVPNRoute struct {
	profile Profile
	secrets Secrets
	// server は、サーバーの Docker の通常回線のアドレスである（設定ファイルの remote）。
	server string
}

// newOpenVPNRoute は、サーバーを立て、それに繋ぐプロファイルとシークレットを作る。
func newOpenVPNRoute(
	t *testing.T, manager *Manager, ctx context.Context, name string, authentication openVPNAuthentication,
) openVPNRoute {
	t.Helper()
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	credentials := newOpenVPNCredentials(t)
	server := startOpenVPNServer(t, manager, ctx, image, openVPNServerName(name), openVPNServerConfig(credentials, authentication))
	route := openVPNRoute{
		profile: Profile{Name: name, Backend: OpenVPN, OpenVPN: &OpenVPNSettings{Servers: []string{server}}},
		secrets: Secrets{OpenVPN: &OpenVPNSecrets{Config: openVPNClientConfig(server, credentials, authentication)}},
		server:  server,
	}
	if authentication == byPassword {
		route.profile.OpenVPN.Username = openVPNTestUsername
		route.secrets.OpenVPN.Password = openVPNTestPassword
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), name) })
	return route
}

// requireOpenVPNLeavesTheServerSettingsAlone は、サーバーが配った既定経路・経路・DNS を
// コンテナが使っていないことを確かめる。トンネルへ向かう経路は、接続先への /32 だけである。
func requireOpenVPNLeavesTheServerSettingsAlone(t *testing.T, manager *Manager, ctx context.Context, profileName string) {
	t.Helper()
	container := manager.containerName(profileName)
	routes, err := manager.docker.output(ctx, "exec", container, "ip", "-4", "route", "show", "dev", "tun0")
	if err != nil {
		t.Fatalf("コンテナの経路を読めない: %v", err)
	}
	if strings.TrimSpace(routes) != openVPNServerTunnelAddress+" scope link" {
		t.Fatalf("tun0 への経路が接続先の /32 だけではない:\n%s", routes)
	}
	defaults, err := manager.docker.output(ctx, "exec", container, "ip", "-4", "route", "show", "default")
	if err != nil || strings.Contains(defaults, "tun0") {
		t.Fatalf("既定経路がトンネルへ向いている: %v\n%s", err, defaults)
	}
	resolvers, err := manager.docker.output(ctx, "exec", container, "cat", "/etc/resolv.conf")
	if err != nil || strings.Contains(resolvers, openVPNPushedResolver) {
		t.Fatalf("サーバーが配った DNS を使っている: %v\n%s", err, resolvers)
	}
	address, err := manager.docker.output(ctx, "exec", container, "ip", "-4", "-o", "address", "show", "dev", "tun0")
	if err != nil || !strings.Contains(address, "/32") {
		t.Fatalf("tun0 のアドレスが /32 ではない: %v\n%s", err, address)
	}
}

// クライアントの証明書で本物の OpenVPN のサーバーへ繋ぎ、トンネルの中の相手へ届く。
// サーバーが配る既定経路・経路・DNS は入れない。
func TestAnOpenVPNTunnelCarriesAConnectionWithACertificate(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenVPN].device())
	route := newOpenVPNRoute(t, manager, ctx, "ovpn-cert", byClientCertificate)

	requireAnswer(t, manager, ctx, dialTarget{profile: route.profile, secrets: route.secrets,
		address: fmt.Sprintf("%s:%d", openVPNServerTunnelAddress, echoPort)}, "tunnelled")

	requireOpenVPNLeavesTheServerSettingsAlone(t, manager, ctx, route.profile.Name)
	// OpenVPN が実際に繋いだサーバーは、接続先にできない。
	connection, err := manager.Dial(ctx, route.profile, route.secrets, fmt.Sprintf("%s:%d", route.server, echoPort))
	if err == nil {
		_ = connection.Close()
		t.Fatal("VPNサーバーそのものへの経路を作った")
	}
	var failure *TargetFailure
	if !errors.As(err, &failure) || failure.Reason != FailureTargetIsServer {
		t.Fatalf("Dial = %v, want %s", err, FailureTargetIsServer)
	}
	status, err := manager.Status(ctx, route.profile.Name)
	if err != nil || status.Tunnel.Interface != "tun0" || status.Tunnel.Backend != string(OpenVPN) {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	// ログには OpenVPN の出力が残る。鍵は伏せてある。
	logs := requireLogs(t, manager, ctx, route.profile.Name, route.secrets)
	if !strings.Contains(logs, "Initialization Sequence Completed") {
		t.Fatalf("ログに OpenVPN の出力が無い:\n%s", logs)
	}
	for _, line := range openVPNInlineLines(route.secrets.OpenVPN.Config) {
		if strings.Contains(logs, line) {
			t.Fatalf("ログに鍵の行が現れた: %s", line)
		}
	}
}

// ユーザー名とパスワードで本物の OpenVPN のサーバーへ繋ぎ、トンネルの中の相手へ届く。
func TestAnOpenVPNTunnelCarriesAConnectionWithAPassword(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenVPN].device())
	route := newOpenVPNRoute(t, manager, ctx, "ovpn-password", byPassword)

	requireAnswer(t, manager, ctx, dialTarget{profile: route.profile, secrets: route.secrets,
		address: fmt.Sprintf("%s:%d", openVPNServerTunnelAddress, echoPort)}, "tunnelled")

	requireOpenVPNLeavesTheServerSettingsAlone(t, manager, ctx, route.profile.Name)
	// パスワードはコンテナの中のファイルにあり、コマンドの引数には無い。
	container := manager.containerName(route.profile.Name)
	arguments, err := manager.docker.output(ctx, "exec", container, "sh", "-c", "cat /proc/[0-9]*/cmdline | tr '\\0' ' '")
	if err != nil || strings.Contains(arguments, openVPNTestPassword) {
		t.Fatalf("パスワードがコマンドの引数に現れた: %v\n%s", err, arguments)
	}
	if logs := requireLogs(t, manager, ctx, route.profile.Name, route.secrets); strings.Contains(logs, openVPNTestPassword) {
		t.Fatalf("ログにパスワードが現れた:\n%s", logs)
	}
}

// パスワードが違えば、サーバーが認証を拒否したことを理由として返す。
func TestAWrongOpenVPNPasswordSaysTheServerRefusedIt(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenVPN].device())
	route := newOpenVPNRoute(t, manager, ctx, "openvpn-refused", byPassword)
	route.secrets.OpenVPN.Password = "wrong-password"

	err := manager.Start(ctx, route.profile, route.secrets)

	requireFailureReason(t, err, FailureOpenVPNAuthentication)
	logs := requireLogs(t, manager, ctx, route.profile.Name, route.secrets)
	if !strings.Contains(logs, "AUTH_FAILED") || strings.Contains(err.Error()+logs, "wrong-password") {
		t.Fatalf("ログが認証の失敗を指していないか、パスワードが現れた: %v\n%s", err, logs)
	}
}

// サーバーの証明書を別の認証局が発行していれば、TLS の失敗を理由として返す。
func TestAnUntrustedOpenVPNServerSaysTheTLSHandshakeFailed(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenVPN].device())
	route := newOpenVPNRoute(t, manager, ctx, "openvpn-untrusted", byClientCertificate)
	// 設定ファイルの <ca> を、サーバーの証明書を発行していない認証局に替える。
	stranger := newOpenVPNCredentials(t)
	config := route.secrets.OpenVPN.Config
	start, end := strings.Index(config, "<ca>"), strings.Index(config, "</ca>")+len("</ca>\n")
	route.secrets.OpenVPN.Config = config[:start] + inline("ca", stranger.ca) + config[end:]

	err := manager.Start(ctx, route.profile, route.secrets)

	requireFailureReason(t, err, FailureOpenVPNTLS)
}

// 応えないサーバーは、締め切りで諦め、応答が無いことを理由として返す。
func TestAnOpenVPNServerThatNeverAnswersSaysSo(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenVPN].device())
	// TEST-NET-1。誰も応答しない。
	const silentServer = "192.0.2.1"
	profile := Profile{Name: "openvpn-silent", Backend: OpenVPN, OpenVPN: &OpenVPNSettings{Servers: []string{silentServer}}}
	secrets := Secrets{OpenVPN: &OpenVPNSecrets{
		Config: openVPNClientConfig(silentServer, newOpenVPNCredentials(t), byClientCertificate),
	}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	err := manager.Start(ctx, profile, secrets)

	requireFailureReason(t, err, FailureOpenVPNNoResponse)
}
