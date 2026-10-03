package vpn

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// fakeDockerReplyVariable は、検査の実行ファイルを偽の docker として起動するときに、
// 書く出力と終了コードを渡す環境変数である。
const fakeDockerReplyVariable = "SSHC_VPN_FAKE_DOCKER_REPLY"

// fakeDockerReply は、偽の docker が書く出力と、終わるときの終了コードである。
//
// sh の script で docker の代わりをする fakeDocker は、Windows では使えない。docker を
// 起動して出力を読む部分（dockerCommand.run）を Windows でも確かめる検査は、検査の
// 実行ファイルそのものをこの答えを書く docker として起動する。
type fakeDockerReply struct {
	// Writes は、書く順に並べた出力である。
	Writes   []fakeDockerWrite `json:"writes"`
	ExitCode int               `json:"exitCode"`
}

// fakeDockerWrite は、偽の docker が1回に書く出力である。
type fakeDockerWrite struct {
	// Stderr なら標準エラーへ、そうでなければ標準出力へ書く。
	Stderr bool   `json:"stderr"`
	Text   string `json:"text"`
}

// writeFakeDockerReply は、偽の docker として答えを書き、終了コードを返す。
func writeFakeDockerReply(encoded string) int {
	var reply fakeDockerReply
	if err := json.Unmarshal([]byte(encoded), &reply); err != nil {
		fmt.Fprintf(os.Stderr, "the fake docker reply is unreadable: %v\n", err)
		return 2
	}
	for _, write := range reply.Writes {
		destination := os.Stdout
		if write.Stderr {
			destination = os.Stderr
		}
		// os.Stdout と os.Stderr は書くたびにそのまま渡すので、書いた順が保たれる。
		_, _ = destination.WriteString(write.Text)
	}
	return reply.ExitCode
}

// fakeDockerProgram は、reply を書いて終わる docker を起動する dockerCommand を返す。
// どの OS でも使える。
func fakeDockerProgram(t *testing.T, reply fakeDockerReply) dockerCommand {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	return dockerCommand{path: self, environment: append(os.Environ(), fakeDockerReplyVariable+"="+string(encoded))}
}
