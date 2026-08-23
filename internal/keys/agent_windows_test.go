//go:build windows

package keys_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"sshc/internal/keys"
	"sshc/internal/platform/windowspipe"
)

// Windows では、エージェントの宛先を環境から変えられない。
//
// Unix の SSH_AUTH_SOCK は OpenSSH 自身の約束であり、端末ごとに別の agent が
// 立つ環境ではそれが唯一の手掛かりである。Windows にその約束は無い。読める変数
// をひとつでも見れば、それは「鍵とパスフレーズを任意のパイプへ渡す方法」に
// なる。この経路を通るのは復号済みの秘密鍵である。
func TestTheWindowsAgentReadsNoEnvironmentToFindItsPipe(t *testing.T) {
	consulted := make([]string, 0, 4)
	dialled := make(chan string, 1)

	adapter := keys.NewAgent(func(name string) (string, bool) {
		consulted = append(consulted, name)
		return `\\.\pipe\an-attacker-would-love-this`, true
	})
	agent, ok := adapter.(keys.Agent)
	if !ok {
		t.Fatalf("NewAgent returned %T, want keys.Agent", adapter)
	}
	agent.Dial = func(_ context.Context, address string) (net.Conn, error) {
		dialled <- address
		return nil, errAgentTestRefused
	}

	agent.Available(context.Background())

	if len(consulted) != 0 {
		t.Errorf("the Windows agent read %v from the environment", consulted)
	}
	select {
	case address := <-dialled:
		if address != windowspipe.AgentPipe {
			t.Errorf("dialled %q, want the fixed %q", address, windowspipe.AgentPipe)
		}
	default:
		t.Fatal("the agent never tried to reach a pipe")
	}
}

// 宛先に資格情報は現れない。エラーもログもこの文字列を含むので、そこに秘密が
// 混ざる余地を残さない。
func TestTheWindowsAgentPipeNameCarriesNothingPrivate(t *testing.T) {
	for _, forbidden := range []string{"passphrase", "password", "secret", "token"} {
		if strings.Contains(strings.ToLower(windowspipe.AgentPipe), forbidden) {
			t.Errorf("the pipe name mentions %q: %q", forbidden, windowspipe.AgentPipe)
		}
	}
	if !strings.HasPrefix(windowspipe.AgentPipe, `\\.\pipe\`) {
		t.Errorf("pipe name = %q, want a local named pipe", windowspipe.AgentPipe)
	}
}

// このテストは繋いだ先で何かをするのではなく、どこへ繋ごうとしたかだけを
// 見る。だから dial は必ず断る。
var errAgentTestRefused = errors.New("refused by the test")
