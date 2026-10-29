package application

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"sshc/internal/storage"
)

// TerminalPreset は、埋め込みターミナルの設定の組である（#467）。「スマホ」「PC」のように
// 画面の種類ごとに名前を付けて同期し、どの組を使うかはブラウザごとに選ぶ。標準の組は
// EmbeddedTerminal そのもので、ここには無い。
//
// 組に入れるのはブラウザが当てる設定だけである。エンジンが当てる設定（セッションの上限、
// 再生バッファ、再接続、接続のログ、開始ディレクトリ、ローカルシェル）と、すべての
// ターミナルに効く許可（OSC 52）は、どのブラウザにも同じ1つの値なので EmbeddedTerminal に置き、
// 組では空のままにする。
type TerminalPreset struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Settings EmbeddedTerminal `json:"settings"`
}

// 設定の組の上限。api/openapi.yaml の TerminalPreset と Metadata.terminalPresets も同じ値を約束する。
const (
	MaxTerminalPresets          = 16
	MaxTerminalPresetNameLength = 80
)

var (
	ErrTerminalPresets        = errors.New("invalid terminal presets")
	ErrTerminalPresetConflict = errors.New("terminal presets changed; reload before saving")
)

// terminalPresetID は組の識別子の形である。default は標準の組が使う。
var terminalPresetID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func validateTerminalPresets(presets []TerminalPreset) error {
	if len(presets) > MaxTerminalPresets {
		return ErrTerminalPresets
	}
	ids := map[string]bool{}
	for _, preset := range presets {
		if err := validateTerminalPreset(preset); err != nil {
			return err
		}
		if ids[preset.ID] {
			return ErrTerminalPresets
		}
		ids[preset.ID] = true
	}
	return nil
}

// validateTerminalPreset は、組の名前と、中身がブラウザの設定だけで範囲の中かを確かめる。
func validateTerminalPreset(preset TerminalPreset) error {
	if !terminalPresetID.MatchString(preset.ID) || preset.ID == "default" ||
		strings.TrimSpace(preset.Name) == "" || utf8.RuneCountInString(preset.Name) > MaxTerminalPresetNameLength ||
		strings.ContainsAny(preset.Name, "\r\n\x00") {
		return ErrTerminalPresets
	}
	if !reflect.DeepEqual(engineTerminalSettings(preset.Settings), EmbeddedTerminal{}) {
		return fmt.Errorf("%w: a preset holds only what the browser applies", ErrTerminalPresets)
	}
	settings := preset.Settings
	return validateEmbeddedTerminal(&settings)
}

// engineTerminalSettings は、設定のうち組に入れないもの（エンジンが当てるものと OSC 52）だけを残す。
func engineTerminalSettings(settings EmbeddedTerminal) EmbeddedTerminal {
	return EmbeddedTerminal{
		MaxSessions:       settings.MaxSessions,
		ScrollbackBytes:   settings.ScrollbackBytes,
		Verbosity:         settings.Verbosity,
		Reconnect:         settings.Reconnect,
		OSC52:             settings.OSC52,
		LocalShellProfile: settings.LocalShellProfile,
		StartDirectory:    settings.StartDirectory,
	}
}

// withoutUnreadableTerminalPresets は、このビルドが使えない組を読み飛ばす。残すと、
// metadata を保存するたびに検証で断られる。
func withoutUnreadableTerminalPresets(metadata *Metadata) {
	kept := metadata.TerminalPresets[:0]
	ids := map[string]bool{}
	for _, preset := range metadata.TerminalPresets {
		if len(kept) >= MaxTerminalPresets || validateTerminalPreset(preset) != nil || ids[preset.ID] {
			continue
		}
		ids[preset.ID] = true
		kept = append(kept, preset)
	}
	if len(kept) == 0 {
		kept = nil
	}
	metadata.TerminalPresets = kept
}

// SetTerminalPresets は、組の一覧をまるごと置き換える。base は画面が読んだときの一覧で、
// そのあとに別の画面やマシンが変えていたら断る。
func (s *Service) SetTerminalPresets(base, presets []TerminalPreset) (SaveResult, error) {
	if err := validateTerminalPresets(presets); err != nil {
		return SaveResult{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	// 空の一覧と無い一覧は、同じ始まりの状態である。
	if !(len(base) == 0 && len(stored.TerminalPresets) == 0) && !reflect.DeepEqual(base, stored.TerminalPresets) {
		return SaveResult{}, ErrTerminalPresetConflict
	}
	stored.TerminalPresets = presets
	if len(presets) == 0 {
		stored.TerminalPresets = nil
	}
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{Operation: "terminal.presets", Changes: []storage.Change{change}})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}
