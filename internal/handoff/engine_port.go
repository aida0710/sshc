package handoff

import "errors"

// engine が待ち受けに使える TCP ポートの範囲。EngineHost と同じく、listen する側と
// handoff の URL を検証する側が同じ値を使う。
const (
	// MinEnginePort より下は OS が特権ポートとして扱う番号なので、engine には使わせない。
	MinEnginePort = 1024
	MaxEnginePort = 65535
)

// ErrEnginePort は、engine の待ち受けに使えないポート番号を表す。
var ErrEnginePort = errors.New("engine port is outside the unprivileged TCP range")

// EnginePort は、この番号を engine の待ち受けに使えるかを報告する。
//
// CLI の --port、設定画面、このマシンの sshc エンジンの設定（engine-settings.json）、
// ブラウザの登録が保存する origin のポート、handoff の URL が、同じ範囲を使う。
func EnginePort(port int) error {
	if port < MinEnginePort || port > MaxEnginePort {
		return ErrEnginePort
	}
	return nil
}
