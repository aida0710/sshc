package application

import (
	"encoding/json"
	"strings"
)

// schema 9 より前の metadata.json は、このマシンの sshc エンジンの設定（受け口のポートと
// Vault の自動ロック）を engine 節に持っていた。schema 9 からは engine-settings.json に
// 置き、同期しない（engine_settings.go）。engine 節を読むのは、このマシンで初めて起動した
// ときの移行（InitialiseEngineSettings）だけである。DecodeMetadata は engine 節を読まない。

// olderMetadataEngineSection は、contents が schema 9 より前の metadata.json なら、その
// engine 節のうち使える値を返す。readable は、contents を metadata.json として読めたかを
// 報告する。空の contents は、metadata.json が無いのと同じく、何も設定されていない。
//
// 範囲の外の値と形の違う値は、手で書き換えたものである。移さずに既定に戻す。前の
// バージョンでも、範囲の外のポートでは起動できず、保存もできなかった。
func olderMetadataEngineSection(contents []byte) (section EngineSettings, readable bool) {
	if len(strings.TrimSpace(string(contents))) == 0 {
		return EngineSettings{}, true
	}
	var older struct {
		SchemaVersion int             `json:"schemaVersion"`
		Engine        json.RawMessage `json:"engine"`
	}
	if err := json.Unmarshal(contents, &older); err != nil {
		return EngineSettings{}, false
	}
	if older.SchemaVersion >= 9 || len(older.Engine) == 0 {
		return EngineSettings{}, true
	}
	if err := json.Unmarshal(older.Engine, &section); err != nil {
		return EngineSettings{}, true
	}
	if validateEngineSettings(EngineSettings{Port: section.Port}) != nil {
		section.Port = 0
	}
	if validateEngineSettings(EngineSettings{VaultAutoLock: section.VaultAutoLock}) != nil {
		section.VaultAutoLock = nil
	}
	return section, true
}
