package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/strictjson"
)

// このマシンの sshc エンジンの設定（受け口のポートと Vault の自動ロック）。
//
// metadata.json とは別のファイルに置き、同期しない（remotesync の neverTravels）。
// metadata.json は同期されるので、そこに置くと、あるマシンで変えたポートが別の
// マシンの受け口とブラウザの登録を変え、自動ロックの選択が別のマシン（共用機でも）
// の Vault にも効いてしまう。
const (
	EngineSettingsFileName = "engine-settings.json"
	// EngineSettingsPathRelative は、ワークスペースからのスラッシュ区切りの相対パスである。
	// remotesync は上の層の application を参照しないので同じ文字列を持ち、app のテストが
	// 照合する。
	EngineSettingsPathRelative = storage.StateDirectoryName + "/" + EngineSettingsFileName

	engineSettingsSchemaVersion = 1
	// engineSettingsTemporaryPrefix は、書き換えに使う一時ファイルの名前の先頭。storage の
	// 一時ファイルと同じ ".sshc-" で始め、書きかけが残っても同期で運ばないようにする。
	engineSettingsTemporaryPrefix = ".sshc-engine-settings-"
)

var (
	ErrEngineSettings = errors.New("engine settings are invalid")

	// ErrEnginePort と ErrVaultAutoLock は ErrEngineSettings の種類。画面に、どちらの
	// 入力を直せばよいかを返すために分ける。
	ErrEnginePort    = fmt.Errorf("%w: port out of range", ErrEngineSettings)
	ErrVaultAutoLock = fmt.Errorf("%w: vault auto lock", ErrEngineSettings)
)

// EngineSettings は、このマシンの sshc エンジンそのものの設定である。
type EngineSettings struct {
	Port          int            `json:"port,omitempty"`
	VaultAutoLock *VaultAutoLock `json:"vaultAutoLock,omitempty"`
}

// VaultAutoLock は、導出済みのVault鍵をmemoryから破棄する条件である。
// restartは自動タイマーを無効にするが、手動ロックとengine終了は引き続き鍵を破棄する。
type VaultAutoLock struct {
	Mode  string `json:"mode"`
	Value int    `json:"value,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

const (
	VaultAutoLockIdle    = "idle"
	VaultAutoLockRestart = "restart"
	VaultAutoLockMinutes = "minutes"
	VaultAutoLockHours   = "hours"
)

// VaultAutoLock の Value（分または時間）の範囲。設定画面は 3 桁までの数で入力させる。
// engine のポートの範囲は、ブラウザの登録も使うので handoff にある。
const (
	MinVaultAutoLockValue = 1
	MaxVaultAutoLockValue = 999
)

// validateEngineSettings は、EngineSettings の範囲を 1 か所で検査する。
// 設定画面の保存と engine-settings.json の読み書きが使う。
func validateEngineSettings(settings EngineSettings) error {
	if settings.Port != 0 && handoff.EnginePort(settings.Port) != nil {
		return fmt.Errorf("%w %d", ErrEnginePort, settings.Port)
	}
	chosen := settings.VaultAutoLock
	if chosen == nil {
		return nil
	}
	switch chosen.Mode {
	case VaultAutoLockRestart:
		if chosen.Value != 0 || chosen.Unit != "" {
			return fmt.Errorf("%w: restart-only auto lock has a duration", ErrVaultAutoLock)
		}
	case VaultAutoLockIdle:
		if chosen.Value < MinVaultAutoLockValue || chosen.Value > MaxVaultAutoLockValue ||
			(chosen.Unit != VaultAutoLockMinutes && chosen.Unit != VaultAutoLockHours) {
			return fmt.Errorf("%w: idle auto lock duration", ErrVaultAutoLock)
		}
	default:
		return fmt.Errorf("%w: mode %q", ErrVaultAutoLock, chosen.Mode)
	}
	return nil
}

// VaultIdleTimeout は保存済みの選択を実行時の時間へ変換する。
// 未設定はfallbackを使い、restartは自動ロックなしを表す0を返す。
func (settings EngineSettings) VaultIdleTimeout(fallback time.Duration) time.Duration {
	chosen := settings.VaultAutoLock
	if chosen == nil {
		return fallback
	}
	if chosen.Mode == VaultAutoLockRestart {
		return 0
	}
	unit := time.Minute
	if chosen.Unit == VaultAutoLockHours {
		unit = time.Hour
	}
	return time.Duration(chosen.Value) * unit
}

// engineSettingsDocument は engine-settings.json の全体である。
type engineSettingsDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	EngineSettings
}

func decodeEngineSettings(contents []byte) (EngineSettings, error) {
	var document engineSettingsDocument
	if err := strictjson.Decode(contents, &document); err != nil {
		return EngineSettings{}, fmt.Errorf("%w: %w", ErrEngineSettings, err)
	}
	if document.SchemaVersion != engineSettingsSchemaVersion {
		return EngineSettings{}, fmt.Errorf("%w: schema version %d", ErrEngineSettings, document.SchemaVersion)
	}
	if err := validateEngineSettings(document.EngineSettings); err != nil {
		return EngineSettings{}, err
	}
	return document.EngineSettings, nil
}

func encodeEngineSettings(settings EngineSettings) ([]byte, error) {
	if err := validateEngineSettings(settings); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(engineSettingsDocument{
		SchemaVersion: engineSettingsSchemaVersion, EngineSettings: settings,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func (s *Service) engineSettingsPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(EngineSettingsPathRelative))
}

// EngineSettings は、このマシンに保存されている sshc エンジンの設定をそのまま返す。
// ファイルが無いか読めなければ、何も設定されていないものとして扱う。
func (s *Service) EngineSettings() EngineSettings {
	contents, exists, err := s.readFile(s.engineSettingsPath())
	if err != nil || !exists {
		return EngineSettings{}
	}
	settings, err := decodeEngineSettings(contents)
	if err != nil {
		return EngineSettings{}
	}
	return settings
}

// SetEngineSettings は、設定をまるごと置き換え、自動ロックの時間をすぐに Vault へ移す。
// 範囲の外は ErrEnginePort か ErrVaultAutoLock で、何も書かずに断る。ポートは起動時に
// しか読まないので、次に sshc エンジンを起動するまで効かない。
//
// いまのファイルは中身を確かめずに置き換える。壊れたファイルも、画面から保存し直せば
// 直せるようにするためである。
func (s *Service) SetEngineSettings(settings EngineSettings) (SaveResult, error) {
	contents, err := encodeEngineSettings(settings)
	if err != nil {
		return SaveResult{}, err
	}
	current, exists, err := s.readFile(s.engineSettingsPath())
	if err != nil {
		return SaveResult{}, err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "engine.settings",
		Changes: []storage.Change{{
			Path: s.engineSettingsPath(), Contents: contents, Precondition: preconditionFor(current, exists),
		}},
	})
	if err != nil {
		return SaveResult{}, err
	}
	s.applyVaultAutoLock()
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}

// InitialiseEngineSettings は、このマシンの sshc エンジンの設定のファイルが無ければ作る。
// sshc エンジンが起動するとき、受け口と Vault の時計がこの設定を読む前に呼ぶ。
//
// 前のバージョンは、この設定を metadata.json の engine 節に書いていた。metadata.json が
// schema 9 より前の形なら、その engine 節をこのマシンの設定として移す。移すのは、
// ファイルがまだ無いとき（このバージョンで初めて起動したとき）だけである。そのあとに
// 同期や履歴の復元で前の形の metadata.json が届いても、別のマシンの設定なので移さない。
// 移すものが無くても空の設定でファイルを作り、移し終えたことを残す。metadata.json が
// JSON として読めなければ、直したあとの起動で移せるよう、まだ作らない。ファイルを
// 読めないなどで失敗したときも何も作らずに理由を返す。sshc エンジンは起動を止めず、
// 既定の設定で動く（internal/app の newEngineServices）。
//
// metadata.json は書き換えない。engine 節は、次に metadata.json を保存するときに消える。
// Vault のロックを解除する前に動くので、変更の履歴を通さずに書く。
func (s *Service) InitialiseEngineSettings() error {
	target := s.engineSettingsPath()
	_, err := s.workspace.FileSystem().Lstat(target)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	metadata, err := s.workspace.FileSystem().ReadFile(s.metadata.Path())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	moved, readable := olderMetadataEngineSection(metadata)
	if !readable {
		return nil
	}
	contents, err := encodeEngineSettings(moved)
	if err != nil {
		return err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	return storage.WriteAtomicFile(s.workspace.FileSystem(), target, engineSettingsTemporaryPrefix,
		storage.FilePermission, contents)
}

// applyVaultAutoLock は、このマシンの自動ロックの時間を Vault の時計へ移す。
// 設定を変える操作（保存、履歴の復元、中断した変更の完了と取り消し）のあとに呼ぶ。
func (s *Service) applyVaultAutoLock() {
	if s.vault == nil {
		return
	}
	s.vault.SetIdleTimeout(s.EngineSettings().VaultIdleTimeout(secret.IdleTimeout))
}
