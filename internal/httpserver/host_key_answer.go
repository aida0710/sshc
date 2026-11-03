package httpserver

import (
	"encoding/json"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/terminal"
)

// ターミナルのセッションと SFTP のタブは、接続が known_hosts に無いホスト鍵について尋ねて
// いるあいだ、同じ形の問い（TerminalHostKeyQuestion）を返し、同じ形の答え
// （TerminalHostKeyAnswer）を受け取る。

// maxFingerprint は、答えに添えるフィンガープリントの長さの上限である（API の TerminalHostKeyAnswer）。
// SHA256 のフィンガープリントは 50 文字なので、十分に余る。
const maxFingerprint = 128

// decodeHostKeyAnswer は、ホスト鍵の問いへの答えを読む。形が違えば false を返す。
func decodeHostKeyAnswer(c *echo.Context) (api.TerminalHostKeyAnswer, bool) {
	// accept を省いた答えを「受け入れない」と読まない。
	var rawRequest struct {
		Fingerprint string          `json:"fingerprint"`
		Accept      json.RawMessage `json:"accept"`
	}
	if err := decodeJSON(c, &rawRequest); err != nil || len(rawRequest.Accept) == 0 ||
		rawRequest.Fingerprint == "" || len(rawRequest.Fingerprint) > maxFingerprint {
		return api.TerminalHostKeyAnswer{}, false
	}
	answer := api.TerminalHostKeyAnswer{Fingerprint: rawRequest.Fingerprint}
	if json.Unmarshal(rawRequest.Accept, &answer.Accept) != nil {
		return api.TerminalHostKeyAnswer{}, false
	}
	return answer, true
}

func describeHostKeyQuestion(question terminal.HostKeyQuestion) *api.TerminalHostKeyQuestion {
	return &api.TerminalHostKeyQuestion{
		Alias: question.Alias, HostName: question.HostName,
		KeyType: question.KeyType, Fingerprint: question.Fingerprint,
	}
}
