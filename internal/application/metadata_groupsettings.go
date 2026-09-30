package application

import (
	"strings"

	"sshc/internal/effective"
)

// グループ設定で変わった、ProxyCommand などの値の持ち方。
//
// ProxyCommand・RemoteCommand・LocalCommand・KnownHostsCommand は、OpenSSH が行の残りを
// そのまま値にする（effective.TakesRestOfLine）。グループ設定は、この値を画面に入力された
// 行の残りのまま 1 つの値で持ち、引用し直さずに書く。
//
// それより前の sshc は、入力を空白で分けて複数の値で保存し、値ごとに引用して書いていた。
// 一重引用符は引用として読まなかったので、`sh -c 'exec nc %h %p'` は
// ["sh", "-c", "'exec", "nc", "%h", "%p'"] として保存されている。metadata schema 9 からは
// 複数の値を保存しない（ValidateMetadata が断る）。前の形を読むのは、schema 9 より前の
// metadata を移行するときだけである（DecodeMetadata）。

// joinSplitRestOfLineSettings は、schema 9 より前の metadata に前の形で保存された
// グループ設定を、前の sshc が書いていた行の残りと同じ 1 つの値にする。
func joinSplitRestOfLineSettings(metadata *Metadata) {
	for groupIndex := range metadata.Groups {
		settings := metadata.Groups[groupIndex].Settings
		for index := range settings {
			setting := &settings[index]
			if !effective.TakesRestOfLine(setting.Keyword) || len(setting.Values) < 2 {
				continue
			}
			setting.Values = []string{legacyRestOfLine(setting.Values)}
		}
	}
}

// legacyRestOfLine は、前の sshc の config.RenderArgument と同じ規則で値を並べる。
// 空の値、空白やタブを含む値、'#' で始まる値だけを二重引用符で囲む。
func legacyRestOfLine(values []string) string {
	written := make([]string, len(values))
	for index, value := range values {
		if value == "" || strings.ContainsAny(value, " \t") || strings.HasPrefix(value, "#") {
			value = `"` + value + `"`
		}
		written[index] = value
	}
	return strings.Join(written, " ")
}
