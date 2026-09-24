package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"sshc/internal/httpserver"
	"sshc/internal/vpn"
	"sshc/internal/vpnrefusal"
)

// VPN 経路は engine が持つ。この CLI は入力を集めて engine へ渡し、返ってきた
// 状態を出すだけである。コンテナも Vault も自分では触らない。

func runVPN(ctx context.Context, called vpnInvocation, environment commandEnvironment) int {
	stateDir, client, stdin, stdout, stderr, terminal :=
		environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr, environment.terminal
	if err := ctx.Err(); err != nil {
		return finishVPNFailure(called, err, environment)
	}
	if called.Action == vpnProxy {
		return runVPNProxy(ctx, called, environment)
	}
	var prompt *os.File
	if called.Action == vpnAdd || called.Action == vpnEdit {
		var err error
		prompt, err = requireSyncSetupTerminal(stdin, stderr, terminal)
		if err != nil {
			return finishSyncFailure(false, errSyncSetupTTY, stdout, stderr)
		}
	}
	prompter := vpnProfilePrompter{ctx: ctx, stdin: stdin, prompt: prompt, terminal: terminal}
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishVPNFailure(called, err, environment)
	}
	defer func() { _ = engine.Close() }()

	// どの操作も、engine は変更後の一覧をそのまま返す。呼び出し側が状態を
	// 取り直す必要はない。
	var overview httpserver.VPNOverview
	switch called.Action {
	case vpnList:
		if err := engine.getJSON(ctx, "/api/v1/vpn", &overview); err != nil {
			return finishVPNFailure(called, err, environment)
		}
	case vpnAdd:
		if err := addVPNProfile(ctx, engine, prompter, called.Name, &overview); err != nil {
			// add はターミナルで対話するので、失敗は人向けに書く。
			return finishVPNFailure(vpnInvocation{Action: vpnAdd, Name: called.Name}, err, environment)
		}
	case vpnEdit:
		if err := editVPNProfile(ctx, engine, prompter, called.Name, &overview); err != nil {
			return finishVPNFailure(vpnInvocation{Action: vpnEdit, Name: called.Name}, err, environment)
		}
	case vpnRemove:
		confirmed, exit := confirmAction(ctx, called.Yes,
			fmt.Sprintf("VPNプロファイル %q を削除しますか？保存済みのシークレットと、このプロファイルを使う接続の設定も削除されます。 [y/N] ",
				safeTerminalCell(called.Name)),
			systemActionConfirmer, stderr)
		if exit != 0 {
			return exit
		}
		if !confirmed {
			fmt.Fprintln(stdout, "キャンセルしました。何も変更していません。")
			return 0
		}
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name), nil, &overview); err != nil {
			return finishVPNFailure(called, err, environment)
		}
	case vpnUp:
		// 経路が立つまで待つあいだ、何も出ないと止まって見える。初回はイメージの
		// 用意だけで分単位になる。
		if !called.JSON {
			fmt.Fprintf(stderr, "%s のVPNに接続しています。初回はコンテナイメージの作成に数分かかることがあります…\n",
				safeTerminalCell(called.Name))
		}
		if err := engine.sendJSON(ctx, http.MethodPost, vpnProfilePath(called.Name)+"/session", nil, &overview); err != nil {
			code := finishVPNFailure(called, err, environment)
			// ログに理由が残るのは、コンテナが経路を用意できなかったときだけである。
			var problem engineProblem
			if !called.JSON && errors.As(err, &problem) && vpnrefusal.HasLogs(problem.Code) {
				fmt.Fprintf(stderr, "詳しくは sshc vpn logs %s でログを確認してください。\n", safeTerminalCell(called.Name))
			}
			return code
		}
	case vpnDown:
		if err := engine.sendJSON(ctx, http.MethodDelete, vpnProfilePath(called.Name)+"/session", nil, &overview); err != nil {
			return finishVPNFailure(called, err, environment)
		}
	case vpnRename:
		body := map[string]string{"name": called.Rename}
		if err := engine.sendJSON(ctx, http.MethodPost,
			vpnProfilePath(called.Name)+"/rename", body, &overview); err != nil {
			return finishVPNFailure(called, err, environment)
		}
	case vpnLogsAction:
		var logs httpserver.VPNLogs
		if err := engine.getJSON(ctx, vpnProfilePath(called.Name)+"/logs", &logs); err != nil {
			return finishVPNFailure(called, err, environment)
		}
		if called.JSON {
			if err := writeCommandEnvelope(stdout, commandEnvelope{
				SchemaVersion: 1, Success: true, Result: logs,
			}); err != nil {
				return 1
			}
			return 0
		}
		for _, line := range strings.Split(strings.TrimRight(logs.Lines, "\n"), "\n") {
			fmt.Fprintln(stdout, safeTerminalCell(line))
		}
		return 0
	case vpnBind, vpnUnbind:
		body := map[string]string{"alias": called.Alias, "profile": called.Name}
		if err := engine.sendJSON(ctx, http.MethodPut, "/api/v1/vpn/bindings", body, &overview); err != nil {
			return finishVPNFailure(called, err, environment)
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

// vpnProfilesPath は、VPN プロファイルの一覧の場所である。作成はここへ送る。
const vpnProfilesPath = "/api/v1/vpn/profiles"

func vpnProfilePath(name string) string {
	return vpnProfilesPath + "/" + url.PathEscape(name)
}

// writeVPNOverview は、一覧と状態を人向けに書く。
func writeVPNOverview(out io.Writer, overview httpserver.VPNOverview) {
	if !overview.Available {
		fmt.Fprintf(out, "このマシンではVPN経路を使用できません。%s\n",
			vpnrefusal.Sentence(vpnrefusal.Refusal{Code: string(overview.Unavailable)}))
		if overview.Detail != "" {
			fmt.Fprintf(out, "詳細: %s\n", safeTerminalCell(overview.Detail))
		}
		fmt.Fprintln(out)
	}
	if len(overview.Profiles) == 0 {
		fmt.Fprintln(out, "VPNプロファイルはありません。sshc vpn add <名前> で作成できます。")
		return
	}
	rows := make([][2]string, 0, len(overview.Profiles)*2)
	for _, session := range overview.Profiles {
		state := "stopped"
		switch {
		case session.RelaySocket != "":
			state = "up"
		case session.Phase != "":
			state = "starting: " + vpnPhaseWord(session.Phase)
		case session.Running:
			state = "starting"
		}
		connections := "-"
		if len(session.Connections) > 0 {
			connections = strings.Join(session.Connections, ", ")
		}
		rows = append(rows,
			[2]string{session.Profile.Name, fmt.Sprintf("%s  %s", session.Profile.Backend, state)},
			[2]string{"", "connections: " + connections},
		)
		if session.Tunnel != nil && session.Tunnel.Interface != "" {
			rows = append(rows, [2]string{"", fmt.Sprintf("tunnel: %s %s since %s",
				session.Tunnel.Interface, session.Tunnel.Address, session.Tunnel.Since)})
		}
		if len(session.Profile.DNS) > 0 {
			rows = append(rows, [2]string{"", "dns: " + strings.Join(session.Profile.DNS, ", ")})
		}
	}
	writeSyncRows(out, rows)
}

// vpnPhaseWord は、経路を用意している段階を人向けの一語に直す。
func vpnPhaseWord(phase vpn.StartPhase) string {
	switch phase {
	case vpn.PhaseImage:
		return "building the image"
	case vpn.PhaseContainer:
		return "starting the container"
	case vpn.PhaseTunnel:
		return "waiting for the tunnel"
	case vpn.PhaseApproval:
		return "waiting for approval on the phone"
	}
	return string(phase)
}
