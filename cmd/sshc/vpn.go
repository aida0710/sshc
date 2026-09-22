package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// VPN 経路は engine が持つ。この CLI は入力を集めて engine へ渡し、返ってきた
// 状態を出すだけである。コンテナも Vault も自分では触らない。

const (
	// maxVPNKeyBytes は、受け取る鍵の長さの上限である。base64 の 32 バイト鍵は
	// 44 文字であり、それを超えるものは形が違う。
	maxVPNKeyBytes = 64
	// defaultTunnelAddress は、トンネル側で名乗るアドレスの初期値である。
	defaultTunnelAddress = "10.0.0.2/32"
)

var errVPNSetupInput = errors.New("vpn profile input is invalid")

// vpnOverview は、engine が返す一覧のうち、この CLI が読む部分である。
type vpnOverview struct {
	Available bool         `json:"available"`
	Detail    string       `json:"detail"`
	Profiles  []vpnSession `json:"profiles"`
}

type vpnSession struct {
	Profile     vpnStoredProfile `json:"profile"`
	Running     bool             `json:"running"`
	Relay       bool             `json:"relay"`
	Connections []string         `json:"connections"`
}

type vpnStoredProfile struct {
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Target  string `json:"target"`
}

func runVPN(ctx context.Context, called vpnInvocation, environment commandEnvironment) int {
	stateDir, client, stdin, stdout, stderr, terminal :=
		environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	var prompt *os.File
	if called.Action == vpnAdd {
		var err error
		prompt, err = requireSyncSetupTerminal(stdin, stderr, terminal)
		if err != nil {
			return finishSyncFailure(false, errSyncSetupTTY, stdout, stderr)
		}
	}
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishSyncFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	// どの操作も、engine は変更後の一覧をそのまま返す。呼び出し側が状態を
	// 取り直す必要はない。
	var overview vpnOverview
	switch called.Action {
	case vpnList:
		if err := engine.getJSON(ctx, "/api/v1/vpn", &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnAdd:
		if err := addVPNProfile(ctx, engine, called.Name, stdin, prompt, terminal, &overview); err != nil {
			return finishSyncFailure(false, err, stdout, stderr)
		}
	case vpnRemove:
		confirmed, exit := confirmAction(ctx, called.Yes,
			fmt.Sprintf("VPNプロファイル %q と、その秘密と、接続への紐付けを削除しますか？ [y/N] ",
				safeTerminalCell(called.Name)),
			systemActionConfirmer, stderr)
		if exit != 0 {
			return exit
		}
		if !confirmed {
			fmt.Fprintln(stdout, "何も変更していません。")
			return 0
		}
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name), nil, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnUp:
		if err := engine.sendJSON(ctx, http.MethodPost, vpnProfilePath(called.Name)+"/session", struct{}{}, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnDown:
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name)+"/session", nil, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	case vpnBind, vpnUnbind:
		body := map[string]string{"alias": called.Alias, "profile": called.Name}
		if err := engine.sendJSON(ctx, http.MethodPut, "/api/v1/vpn/bindings", body, &overview); err != nil {
			return finishSyncFailure(called.JSON, err, stdout, stderr)
		}
	}
	if called.JSON {
		if err := writeCommandEnvelope(stdout, commandEnvelope{
			SchemaVersion: 1, Success: true, Result: overview,
		}); err != nil {
			return 1
		}
		return 0
	}
	writeVPNOverview(stdout, overview)
	return 0
}

func vpnProfilePath(name string) string {
	return "/api/v1/vpn/profiles/" + url.PathEscape(name)
}

// addVPNProfile は、プロファイルひとつ分の入力を集めて engine へ渡す。
//
// 秘密は引数にも環境変数にも置かない。端末から no-echo で読み、送ったあとに
// その場で消す。
func addVPNProfile(
	ctx context.Context, engine *engineAPI, name string,
	stdin, prompt *os.File, terminal passwordTerminal, overview *vpnOverview,
) error {
	backend, err := promptVisibleSetup(ctx, stdin, prompt, setupVisibleLabel("Backend", "wireguard"), "wireguard")
	if err != nil {
		return err
	}
	if backend != "wireguard" {
		return fmt.Errorf("%w: いまは wireguard だけを扱えます", errVPNSetupInput)
	}
	server, err := promptVisibleSetup(ctx, stdin, prompt, "VPN server (host:port): ", "")
	if err != nil {
		return err
	}
	peerKey, err := promptVisibleSetup(ctx, stdin, prompt, "Peer public key: ", "")
	if err != nil {
		return err
	}
	address, err := promptVisibleSetup(ctx, stdin, prompt,
		setupVisibleLabel("Tunnel address", defaultTunnelAddress), defaultTunnelAddress)
	if err != nil {
		return err
	}
	target, err := promptVisibleSetup(ctx, stdin, prompt, "Target through the VPN (host:port): ", "")
	if err != nil {
		return err
	}
	privateKey, err := promptMaskedPassword(ctx, stdin, prompt, terminal, "Private key: ")
	if err != nil {
		zeroBytes(privateKey)
		return err
	}
	defer zeroBytes(privateKey)
	if server == "" || peerKey == "" || address == "" || target == "" || len(privateKey) == 0 {
		return errVPNSetupInput
	}
	payload, err := buildVPNProfilePayload(vpnProfileFields{
		name: name, backend: backend, target: target,
		server: server, peerPublicKey: peerKey, address: address,
	}, privateKey)
	if err != nil {
		return err
	}
	// payload は秘密鍵を含む。sendSecretJSON が送り終えた本文を消す。
	return engine.sendSecretJSON(ctx, http.MethodPut, vpnProfilePath(name), payload, overview)
}

type vpnProfileFields struct {
	name          string
	backend       string
	target        string
	server        string
	peerPublicKey string
	address       string
}

// buildVPNProfilePayload は、保存要求の本文を組み立てる。
//
// 秘密鍵だけは Go の文字列にしない。文字列にすると、送ったあとに消せる場所が
// なくなる。鍵は base64 の字しか持たないので、その形を先に確かめてから、
// そのまま本文へ写す。
func buildVPNProfilePayload(fields vpnProfileFields, privateKey []byte) ([]byte, error) {
	if len(privateKey) > maxVPNKeyBytes || !base64KeyBytes(privateKey) {
		return nil, fmt.Errorf("%w: 秘密鍵の形が違います", errVPNSetupInput)
	}
	profile := map[string]any{
		"name": fields.name, "backend": fields.backend, "target": fields.target,
		"wireguard": map[string]string{
			"server": fields.server, "peerPublicKey": fields.peerPublicKey, "address": fields.address,
		},
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	prefix := append([]byte(`{"profile":`), encoded...)
	prefix = append(prefix, []byte(`,"secrets":{"wireguardPrivateKey":"`)...)
	payload := make([]byte, 0, len(prefix)+len(privateKey)+4)
	payload = append(payload, prefix...)
	payload = append(payload, privateKey...)
	return append(payload, []byte(`"}}`)...), nil
}

// base64KeyBytes は、鍵が base64 の字だけでできているかを報告する。
func base64KeyBytes(key []byte) bool {
	if len(key) == 0 {
		return false
	}
	for _, character := range key {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && character != '+' && character != '/' && character != '=' {
			return false
		}
	}
	return true
}

// writeVPNOverview は、一覧と状態を人向けに書く。
func writeVPNOverview(out io.Writer, overview vpnOverview) {
	if !overview.Available {
		fmt.Fprintf(out, "この機械ではVPN経路を作れません: %s\n\n", safeTerminalCell(overview.Detail))
	}
	if len(overview.Profiles) == 0 {
		fmt.Fprintln(out, "VPNプロファイルはありません。sshc vpn add <名前> で作成します。")
		return
	}
	rows := make([][2]string, 0, len(overview.Profiles)*2)
	for _, session := range overview.Profiles {
		state := "stopped"
		switch {
		case session.Relay:
			state = "up"
		case session.Running:
			state = "starting"
		}
		connections := "-"
		if len(session.Connections) > 0 {
			connections = strings.Join(session.Connections, ", ")
		}
		rows = append(rows,
			[2]string{session.Profile.Name, fmt.Sprintf("%s  %s  %s",
				session.Profile.Backend, session.Profile.Target, state)},
			[2]string{"", "connections: " + connections},
		)
	}
	writeSyncRows(out, rows)
}
