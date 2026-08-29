package httpserver

import (
	"context"
	"errors"

	"sshc/internal/application"
	"sshc/internal/keys"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// Connector は、alias ひとつ分の対話セッションを開く。
//
// 外部の ssh は起こさない。組み立てるのは合成の根（internal/app）であり、
// 鍵も vault も known_hosts もそこで一度だけ配線される。二箇所で組み立てると、
// 片方だけが vault を見る日が来る。
type Connector func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error)

// AgentConnector opens a fixed coding-agent resume program on the same SSH
// destination. kind/reference are engine-owned values, never browser command text.
type AgentConnector func(ctx context.Context, alias string, kind terminal.AgentKind, reference string, size terminal.Size) (terminal.Process, error)

// connectProblem は、接続を組み立てられなかった理由を通信形式に変える。
func connectProblem(err error) (string, bool) {
	var unresolvable *application.ErrUnresolvable
	switch {
	case errors.As(err, &unresolvable):
		return "alias_unresolvable", true
	case errors.Is(err, sshclient.ErrProxyCommandWithJump):
		return "proxy_command_with_jump", true
	case errors.Is(err, sshclient.ErrJumpDepth):
		return "jump_depth_exceeded", true
	case errors.Is(err, sshclient.ErrNoHostName):
		return "alias_unresolvable", true
	case errors.Is(err, sshclient.ErrHostKeyUnknown):
		return "host_key_unknown", true
	case errors.Is(err, sshclient.ErrHostKeyChanged):
		return "host_key_changed", true
	case errors.Is(err, sshclient.ErrHostKeyRevoked):
		return "host_key_revoked", true
	case errors.Is(err, sshclient.ErrNoIdentity):
		return "identity_unavailable", true
	case errors.Is(err, sshclient.ErrNoAuthMethod):
		return "authentication_unavailable", true
	case errors.Is(err, sshclient.ErrPromptAborted):
		return "authentication_cancelled", true
	case errors.Is(err, keys.ErrPassphraseRequired), errors.Is(err, keys.ErrWrongPassphrase):
		return "key_passphrase_required", true
	}
	return "", false
}
