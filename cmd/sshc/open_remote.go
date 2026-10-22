package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sshc/internal/httpserver"
	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

// openRemoteTimeout は、`sshc open <alias>` がこのマシンのエンジンの答えを待つ上限である。
// エンジンはその間に、接続先へ SSH で繋ぎ（ConnectTimeout が無ければ 30 秒）、接続先で
// sshc open を走らせる（EntranceLimit）。connectTimeout は、このマシンのエンジンとの
// やり取りの分である。
const openRemoteTimeout = sshclient.DefaultTimeout + remoteengine.EntranceLimit + connectTimeout

// runOpenRemote は、alias の SSH の接続先で動いているエンジンを、このマシンのエンジンの
// 中継を通して開く URL を出力する。open が true の場合だけ URL をブラウザへ渡す。
func runOpenRemote(ctx context.Context, alias string, environment commandEnvironment, open bool) int {
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(environment.stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return exitUsage
	}
	body, err := json.Marshal(map[string]string{"alias": alias})
	if err != nil {
		fmt.Fprintf(environment.stderr, "sshc: %v\n", err)
		return exitFailure
	}
	var client http.Client
	if environment.client != nil {
		client = *environment.client
	}
	client.Timeout = openRemoteTimeout
	return openUIURL(ctx, environment, uiURLRequest{
		call:    handoffCall{method: http.MethodPost, path: httpserver.RemoteEnginePath, body: bytes.NewReader(body)},
		client:  &client,
		explain: func(refusal engineProblem) string { return remoteEngineRefusal(alias, refusal) },
	}, open)
}

// remoteEngineRefusal は、接続先のエンジンを開けなかった理由を、直し方と一緒に述べる。
func remoteEngineRefusal(alias string, refusal engineProblem) string {
	switch refusal.Code {
	case "remote_engine_connect_failed":
		switch refusal.Reason {
		case "host_key_unknown":
			return fmt.Sprintf("%s is not in known_hosts; connect once with sshc ssh %s to decide about its host key", alias, alias)
		case "prompt_unavailable":
			return fmt.Sprintf("connecting to %s needs a passphrase, password or one-time code that is not saved; save it in Connections, or unlock the vault with sshc vault unlock", alias)
		}
		return fmt.Sprintf("could not connect to %s; run sshc ssh %s to see why", alias, alias)
	case "remote_engine_not_installed":
		return fmt.Sprintf("sshc was not found on %s; install sshc there first", alias)
	case "remote_engine_forwarding_refused":
		return fmt.Sprintf("the SSH server on %s refused to forward a connection to its sshc engine; allow it with AllowTcpForwarding local (or yes) in its sshd_config", alias)
	case "remote_engine_no_entrance":
		return fmt.Sprintf("sshc on %s did not return a UI URL; run sshc open --no-browser on %s to see why", alias, alias)
	}
	return refusedRequestError(refusal).Error()
}
