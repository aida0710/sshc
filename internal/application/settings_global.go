package application

import (
	"fmt"
	"regexp"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// GlobalSettings は全体の設定、つまりどのマシン・どのプリセットでも同じ値の設定である。
// 何を全体に置くかは利用者が決めた（docs/design.md の「設定の置き場所」）。
type GlobalSettings struct {
	// Language は画面の言語（en・ja）である。空はブラウザの言語に従う。
	Language string `json:"language,omitempty"`
	// Terminal と FileTransfers は、ターミナルと SFTP の転送の設定のうち全体に置くものである。
	Terminal      *GlobalTerminalSettings     `json:"terminal,omitempty"`
	FileTransfers *GlobalFileTransferSettings `json:"fileTransfers,omitempty"`
	// Backgrounds は背景画像のライブラリの容量である。画像そのものも全体で1つなので、ここに置く。
	Backgrounds *BackgroundSettings `json:"backgrounds,omitempty"`
	// UpdateNoticeFoldedRelease は、更新のお知らせを折りたたんだときのリリースのタグである。
	// それより新しいリリースが出ると、お知らせはまた開く。
	UpdateNoticeFoldedRelease string `json:"updateNoticeFoldedRelease,omitempty"`
}

// GlobalTerminalSettings は、埋め込みターミナルの設定のうち全体に置くものである。
type GlobalTerminalSettings struct {
	// Reconnect は、輸送が落ちたときに繋ぎ直しを試みる回数である。nil は既定、0 は繋ぎ直さない。
	Reconnect *int `json:"reconnect,omitempty"`
	// Verbosity は、接続の途中経過をどこまでターミナルへ書くかである。0 は書かない。
	Verbosity int `json:"verbosity,omitempty"`
	// OSC52 は、すべてのターミナルで、リモートのプログラムが OSC 52 でクリップボードへ書き込むのを許す。
	OSC52 bool `json:"osc52,omitempty"`
}

// GlobalFileTransferSettings は、SFTP の転送の設定のうち全体に置くものである。
type GlobalFileTransferSettings struct {
	ClearCompletedAfterSeconds int  `json:"clearCompletedAfterSeconds,omitempty"`
	AutoReconnect              bool `json:"autoReconnect,omitempty"`
	MaxReconnectAttempts       int  `json:"maxReconnectAttempts,omitempty"`
}

// globalLanguages は画面が持つ言語である（web/src/i18n/locale.ts の locales）。
var globalLanguages = map[string]bool{"en": true, "ja": true}

// releaseTagPattern は、更新のお知らせが比べるリリースの名前の形である。リリースの名前は更新の
// 確認が返すまま（v0.59.0、プレリリースの -rc.1 なども）なので、使う文字と長さだけを確かめる。
var releaseTagPattern = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,64}$`)

func validateGlobalSettings(settings *GlobalSettings) error {
	if settings == nil {
		return nil
	}
	if settings.Language != "" && !globalLanguages[settings.Language] {
		return fmt.Errorf("%w: language %q", ErrMetadataTerminal, settings.Language)
	}
	if err := validateGlobalTerminalSettings(settings.Terminal); err != nil {
		return err
	}
	if transfers := settings.FileTransfers; transfers != nil &&
		(transfers.ClearCompletedAfterSeconds < 0 || transfers.MaxReconnectAttempts < 0) {
		return fmt.Errorf("%w: file transfers", ErrMetadataTerminal)
	}
	if backgrounds := settings.Backgrounds; backgrounds != nil &&
		(backgrounds.CapacityMiB < MinBackgroundCapacityMiB || backgrounds.CapacityMiB > MaxBackgroundCapacityMiB) {
		return fmt.Errorf("%w: background capacity %d", ErrMetadataTerminal, backgrounds.CapacityMiB)
	}
	if settings.UpdateNoticeFoldedRelease != "" && !releaseTagPattern.MatchString(settings.UpdateNoticeFoldedRelease) {
		return fmt.Errorf("%w: updateNoticeFoldedRelease", ErrMetadataTerminal)
	}
	return nil
}

func validateGlobalTerminalSettings(settings *GlobalTerminalSettings) error {
	if settings == nil {
		return nil
	}
	// 上限は接続ログの段の数で決まるので、sshclient.MaxVerbosity を見る。api/openapi.yaml の
	// GlobalTerminalSettings の verbosity の maximum もこの値にそろえる。
	if settings.Verbosity < 0 || settings.Verbosity > sshclient.MaxVerbosity {
		return fmt.Errorf("%w: verbosity %d", ErrMetadataTerminal, settings.Verbosity)
	}
	if settings.Reconnect != nil && (*settings.Reconnect < 0 || *settings.Reconnect > terminal.MaxReconnects) {
		return fmt.Errorf("%w: reconnect %d", ErrMetadataTerminal, *settings.Reconnect)
	}
	return nil
}

// withoutUnreadableGlobalSettings は、このビルドが読めない全体の設定を既定へ戻す。metadata は
// 同期で別のマシンから届くので、ここで戻さないと、画面が metadata をまるごと読めなくなる。
func withoutUnreadableGlobalSettings(metadata *Metadata) {
	if validateGlobalSettings(metadata.Settings) != nil {
		metadata.Settings = nil
	}
}

// SetGlobalSettings は、全体の設定をまるごと置き換える。base は画面が読んだときの値で、
// そのあとに別の画面やマシンが変えていたら ErrSettingsChanged で断る。
func (s *Service) SetGlobalSettings(base, settings GlobalSettings) (SaveResult, error) {
	if err := validateGlobalSettings(&settings); err != nil {
		return SaveResult{}, err
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if !sameSettings(base, globalSettingsOf(stored)) {
		return SaveResult{}, ErrSettingsChanged
	}
	stored.Settings = storedGlobalSettings(settings)
	return s.commitMetadata(stored, precondition, "settings.global")
}

// globalSettingsOf は、書かれていない全体の設定を空の値として読む。
func globalSettingsOf(metadata Metadata) GlobalSettings {
	if metadata.Settings == nil {
		return GlobalSettings{}
	}
	return *metadata.Settings
}

// storedGlobalSettings は、何も設定していない全体の設定を「書かれていない」に潰す。空の節を
// 残すと、次に読む者は何か設定されていると思う。
func storedGlobalSettings(settings GlobalSettings) *GlobalSettings {
	if sameSettings(settings, GlobalSettings{}) {
		return nil
	}
	return &settings
}
