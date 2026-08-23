//go:build unix

package keys_test

import (
	"context"
	"testing"

	"sshc/internal/keys"
)

// agent_windows_test.go の対になる側。
//
// Unix では SSH_AUTH_SOCK が唯一の手掛かりである。端末ごとに別の agent が
// 立つ環境があり、どれに通信するかを決めているのはその変数だけなので、ここでは
// 読まなければ何にも届かない。Windows にその約束は無く、あちらは固定の名前を
// 使う。同じ検査を両方で走らせると、落ちるだけでなく「環境が宛先を決める」
// という誤りがどちらかに残る。
func TestTheUnixAgentFindsItsSocketThroughTheEnvironment(t *testing.T) {
	socket, _ := runAgent(t)
	consulted := make([]string, 0, 2)

	adapter := keys.NewAgent(func(name string) (string, bool) {
		consulted = append(consulted, name)
		if name != "SSH_AUTH_SOCK" {
			return "", false
		}
		return socket, true
	})

	if !adapter.Available(context.Background()) {
		t.Fatal("the agent named by SSH_AUTH_SOCK was reported unreachable")
	}
	found := false
	for _, name := range consulted {
		if name == "SSH_AUTH_SOCK" {
			found = true
		}
	}
	if !found {
		t.Errorf("the Unix agent read %v and never asked for SSH_AUTH_SOCK", consulted)
	}
}

// 変数が無いことと、その先に誰かが居ないことは同じ扱いでよい。死んだ端末が
// 残した SSH_AUTH_SOCK はいつまでも残るので、どちらも開いてみて返す。
func TestTheUnixAgentWithoutTheVariableIsUnavailable(t *testing.T) {
	adapter := keys.NewAgent(func(string) (string, bool) { return "", false })

	if adapter.Available(context.Background()) {
		t.Error("an agent with no SSH_AUTH_SOCK reported itself available")
	}
}
