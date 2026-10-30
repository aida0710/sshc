package application

import (
	"encoding/json"
	"fmt"
)

// schema 9 の metadata.json を、一度だけ schema 10 へ移す。全部のマシンが schema 10 のビルドに
// なったら、このファイルごと消す（docs/design.md の「設定の置き場所」）。
//
// 移した結果は、schema 9 のバイト列だけで決まる（ホスト名・時刻・乱数・このマシンだけの
// ファイルを混ぜない）。同期している2台がそれぞれ同じ schema 9 から移すと、同じバイト列になり、
// 同期はそれを衝突と見なさない（remotesync の PlanEntries は両側が同じなら飛ばす）。

const previousMetadataSchemaVersion = 9

// embeddedTerminalSchema9 は、schema 9 の embeddedTerminal である。全体の設定へ移る3つを持つ。
type embeddedTerminalSchema9 struct {
	EmbeddedTerminal
	Verbosity int  `json:"verbosity,omitempty"`
	Reconnect *int `json:"reconnect,omitempty"`
	OSC52     bool `json:"osc52,omitempty"`
}

// fileTransfersSchema9 は、schema 9 の fileTransfers である。全体の設定へ移る3つと、マシンの
// 今の状態だった processingStopped を持つ。
type fileTransfersSchema9 struct {
	FileTransferSettings
	ClearCompletedAfterSeconds int  `json:"clearCompletedAfterSeconds,omitempty"`
	ProcessingStopped          bool `json:"processingStopped,omitempty"`
	AutoReconnect              bool `json:"autoReconnect,omitempty"`
	MaxReconnectAttempts       int  `json:"maxReconnectAttempts,omitempty"`
}

// settingsSchema9 は、schema 9 で設定を持っていた節である。ほかの節（接続先・グループ・VPN・
// ショートカットのプリセット）は schema 10 でも同じ形なので、そのまま読む。
type settingsSchema9 struct {
	EmbeddedTerminal *embeddedTerminalSchema9 `json:"embeddedTerminal"`
	FileTransfers    *fileTransfersSchema9    `json:"fileTransfers"`
	Backgrounds      *BackgroundSettings      `json:"backgrounds"`
	Appearance       *AppearanceSettings      `json:"appearance"`
	TerminalPresets  []struct {
		ID       string                  `json:"id"`
		Name     string                  `json:"name"`
		Settings embeddedTerminalSchema9 `json:"settings"`
	} `json:"terminalPresets"`
}

// settingsSectionsSchema9 は、schema 10 に無い schema 9 の節の名前である。
var settingsSectionsSchema9 = []string{"embeddedTerminal", "fileTransfers", "backgrounds", "appearance", "terminalPresets"}

// migrateMetadataFromSchema9 は、schema 9 の metadata.json を schema 10 の形にする。
//
//   - 再接続・接続のログ・OSC 52、転送の自動再接続と完了した転送を消すまで、背景画像の容量は全体の設定へ。
//   - それ以外の設定とさし色は標準のプリセットへ。
//   - schema 9 の設定の組はプリセットへ。組に無かったエンジンの設定（上限・開始ディレクトリ・シェル）と
//     転送の設定とさし色は、標準のプリセットの値で埋める（schema 9 ではそれが効いていたので）。
//   - 転送の処理を止めていたか（processingStopped）は捨てる。マシンの今の状態で、schema 9 からは
//     どのマシンのものか分からない。
func migrateMetadataFromSchema9(contents []byte) (Metadata, error) {
	var old settingsSchema9
	if err := json.Unmarshal(contents, &old); err != nil {
		return Metadata{}, err
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(contents, &sections); err != nil {
		return Metadata{}, err
	}
	for _, name := range settingsSectionsSchema9 {
		delete(sections, name)
	}
	sections["schemaVersion"] = json.RawMessage(fmt.Sprint(MetadataSchemaVersion))
	rest, err := json.Marshal(sections)
	if err != nil {
		return Metadata{}, err
	}
	metadata, err := DecodeMetadata(rest)
	if err != nil {
		return Metadata{}, err
	}

	global := GlobalSettings{Backgrounds: old.Backgrounds}
	standard := SettingsPreset{ID: DefaultPresetID, Appearance: old.Appearance}
	if terminal := old.EmbeddedTerminal; terminal != nil {
		global.Terminal = storedSection(GlobalTerminalSettings{Reconnect: terminal.Reconnect, Verbosity: terminal.Verbosity, OSC52: terminal.OSC52})
		standard.Terminal = storedSection(terminal.EmbeddedTerminal)
	}
	if transfers := old.FileTransfers; transfers != nil {
		global.FileTransfers = storedSection(GlobalFileTransferSettings{
			ClearCompletedAfterSeconds: transfers.ClearCompletedAfterSeconds,
			AutoReconnect:              transfers.AutoReconnect,
			MaxReconnectAttempts:       transfers.MaxReconnectAttempts,
		})
		standard.FileTransfers = storedSection(transfers.FileTransferSettings)
	}
	metadata.Settings = storedGlobalSettings(global)
	metadata.Presets = []SettingsPreset{standard}
	for _, set := range old.TerminalPresets {
		terminal := set.Settings.EmbeddedTerminal
		if standard.Terminal != nil {
			terminal.MaxSessions = standard.Terminal.MaxSessions
			terminal.ScrollbackBytes = standard.Terminal.ScrollbackBytes
			terminal.LocalShellProfile = standard.Terminal.LocalShellProfile
			terminal.StartDirectory = standard.Terminal.StartDirectory
		}
		metadata.Presets = append(metadata.Presets, SettingsPreset{
			ID: set.ID, Name: set.Name, Appearance: standard.Appearance,
			Terminal: storedSection(terminal), FileTransfers: standard.FileTransfers,
		})
	}
	withoutUnreadableGlobalSettings(&metadata)
	withoutUnreadablePresets(&metadata)
	return metadata, nil
}

// storedSection は、何も設定していない節を「書かれていない」に潰す。
func storedSection[T any](section T) *T {
	var empty T
	if sameSettings(section, empty) {
		return nil
	}
	return &section
}

// migrateMetadataFromSchema9IfOld は、schema 9 のファイルだけを移す。ほかの schema は今までどおり
// ErrMetadataVersion で断る。
func migrateMetadataFromSchema9IfOld(contents []byte) (Metadata, error) {
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(contents, &version); err != nil {
		return Metadata{}, err
	}
	if version.SchemaVersion != previousMetadataSchemaVersion {
		return Metadata{}, ErrMetadataVersion
	}
	return migrateMetadataFromSchema9(contents)
}
