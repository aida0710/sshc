package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SettingsPreset はプリセット、つまり「スマホ」「PC」のように名前を付けて同期し、マシンごとに
// どれを使うか選ぶ設定のまとまりである。何をプリセットに入れるかは利用者が決めた
// （docs/design.md の「設定の置き場所」）。
type SettingsPreset struct {
	ID string `json:"id"`
	// Name は表示する名前である。標準のプリセットは名前を持たず、画面が「標準」と出す。
	Name          string                `json:"name,omitempty"`
	Appearance    *AppearanceSettings   `json:"appearance,omitempty"`
	Terminal      *EmbeddedTerminal     `json:"terminal,omitempty"`
	FileTransfers *FileTransferSettings `json:"fileTransfers,omitempty"`
	VaultAutoLock *VaultAutoLock        `json:"vaultAutoLock,omitempty"`
	Interface     *InterfaceSettings    `json:"interface,omitempty"`
}

// InterfaceSettings は、画面の操作と並び方のうちプリセットに入るものである。エンジンは使わず、
// 画面が読む。
type InterfaceSettings struct {
	// TouchMode は、ターミナルがタッチをどう扱うか（touch・mouse）である。空は自動で判定する。
	TouchMode string `json:"touchMode,omitempty"`
	// NotificationSound は、通知のときに鳴らす音である。nil は既定の音。
	NotificationSound *NotificationSound `json:"notificationSound,omitempty"`
	// KeepScreenAwake は、ターミナルを表示している間、画面を消さない。
	KeepScreenAwake bool `json:"keepScreenAwake,omitempty"`
	// ShortcutPreset は、使うショートカットのプリセットの ID である。空は既定の割り当て。
	ShortcutPreset string `json:"shortcutPreset,omitempty"`
	// NavigationWidth、ConnectionListWidth、ConnectionGroupsWidth は、ドラッグで決めた欄の幅
	// （px）である。0 は既定の幅。範囲は画面が持ち、ここでは大きさの桁だけを確かめる。
	NavigationWidth       int `json:"navigationWidth,omitempty"`
	ConnectionListWidth   int `json:"connectionListWidth,omitempty"`
	ConnectionGroupsWidth int `json:"connectionGroupsWidth,omitempty"`
	// HomeView は Home の接続先の表示形式（panel・list）である。空は既定。
	HomeView string `json:"homeView,omitempty"`
}

// NotificationSound は、通知のときに鳴らす音の種類と大きさである。
type NotificationSound struct {
	Sound  string `json:"sound"`
	Volume int    `json:"volume"`
}

// DefaultPresetID は標準のプリセットの ID である。標準のプリセットは消せず、名前も持たない。
// 使うプリセットを選んでいないマシンと、選んだプリセットが消えたマシンは、これを使う。
const DefaultPresetID = "default"

// プリセットの上限。api/openapi.yaml の SettingsPreset と Metadata.presets も同じ値を約束する。
const (
	MaxPresets          = 16
	MaxPresetNameLength = 80
	// maxColumnWidth は、欄の幅として受け取る上限である。どの画面より広い幅は、壊れた値として断る。
	maxColumnWidth = 4096
)

var (
	ErrPresets = errors.New("invalid presets")
	// ErrSettingsChanged は、画面が読んだあとに、ほかの画面かマシンが同じ設定を変えたことを表す。
	// 画面は読み直してからやり直す。
	ErrSettingsChanged = errors.New("settings changed; reload before saving")
	// ErrUnknownPreset は、無いプリセットを指したことを表す。
	ErrUnknownPreset = errors.New("unknown preset")
)

// presetIDPattern はプリセットの ID の形である。
var presetIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

var (
	touchModes         = map[string]bool{"touch": true, "mouse": true}
	notificationSounds = map[string]bool{"none": true, "gentle": true, "bell": true, "pulse": true}
	homeViews          = map[string]bool{"panel": true, "list": true}
)

// maxNotificationVolume は、通知の音の大きさ（％）の上限である。
const maxNotificationVolume = 100

func validatePresets(presets []SettingsPreset) error {
	if len(presets) == 0 || presets[0].ID != DefaultPresetID || len(presets) > MaxPresets {
		return fmt.Errorf("%w: the standard preset comes first", ErrPresets)
	}
	ids := map[string]bool{}
	for _, preset := range presets {
		if err := validatePreset(preset); err != nil {
			return err
		}
		if ids[preset.ID] {
			return fmt.Errorf("%w: duplicate id", ErrPresets)
		}
		ids[preset.ID] = true
	}
	return nil
}

// validatePreset は、プリセットの名前と、中身が範囲の中かを確かめる。
func validatePreset(preset SettingsPreset) error {
	if !presetIDPattern.MatchString(preset.ID) {
		return fmt.Errorf("%w: id", ErrPresets)
	}
	if preset.ID == DefaultPresetID {
		if preset.Name != "" {
			return fmt.Errorf("%w: the standard preset has no name", ErrPresets)
		}
	} else if strings.TrimSpace(preset.Name) == "" || utf8.RuneCountInString(preset.Name) > MaxPresetNameLength ||
		strings.ContainsAny(preset.Name, "\r\n\x00") {
		return fmt.Errorf("%w: name", ErrPresets)
	}
	if err := validateAppearanceSettings(preset.Appearance); err != nil {
		return err
	}
	if err := validateEmbeddedTerminal(preset.Terminal); err != nil {
		return err
	}
	if err := validateFileTransferSettings(preset.FileTransfers); err != nil {
		return err
	}
	if err := validateVaultAutoLock(preset.VaultAutoLock); err != nil {
		return err
	}
	return validateInterfaceSettings(preset.Interface)
}

// validateFileTransferSettings は、負の数だけを断る。上限は転送キュー（sftp.TransferSettings）が
// 当てるときに丸めるので、ここでは持たない。
func validateFileTransferSettings(settings *FileTransferSettings) error {
	if settings == nil {
		return nil
	}
	if settings.MaxConcurrent < 0 || settings.LargeFileThresholdBytes < 0 || settings.LargeFileParallelism < 0 ||
		settings.LargeFileChunkBytes < 0 || settings.SpeedLimitBytesPerSecond < 0 {
		return fmt.Errorf("%w: file transfers", ErrPresets)
	}
	return nil
}

func validateInterfaceSettings(settings *InterfaceSettings) error {
	if settings == nil {
		return nil
	}
	if settings.TouchMode != "" && !touchModes[settings.TouchMode] {
		return fmt.Errorf("%w: touchMode", ErrPresets)
	}
	if sound := settings.NotificationSound; sound != nil &&
		(!notificationSounds[sound.Sound] || sound.Volume < 0 || sound.Volume > maxNotificationVolume) {
		return fmt.Errorf("%w: notificationSound", ErrPresets)
	}
	if settings.ShortcutPreset != "" && !presetIDPattern.MatchString(settings.ShortcutPreset) {
		return fmt.Errorf("%w: shortcutPreset", ErrPresets)
	}
	for _, width := range []int{settings.NavigationWidth, settings.ConnectionListWidth, settings.ConnectionGroupsWidth} {
		if width < 0 || width > maxColumnWidth {
			return fmt.Errorf("%w: column width", ErrPresets)
		}
	}
	if settings.HomeView != "" && !homeViews[settings.HomeView] {
		return fmt.Errorf("%w: homeView", ErrPresets)
	}
	return nil
}

// withoutUnreadablePresets は、このビルドが使えないプリセットを読み飛ばし、標準のプリセットを
// 先頭に置く。読めない標準のプリセットは既定の値に戻す。残すと、metadata を保存するたびに検証で
// 断られる。
func withoutUnreadablePresets(metadata *Metadata) {
	standard := SettingsPreset{ID: DefaultPresetID}
	kept := []SettingsPreset{}
	ids := map[string]bool{DefaultPresetID: true}
	for _, preset := range metadata.Presets {
		if preset.ID == DefaultPresetID {
			if validatePreset(preset) == nil {
				standard = preset
			}
			continue
		}
		if len(kept) >= MaxPresets-1 || validatePreset(preset) != nil || ids[preset.ID] {
			continue
		}
		ids[preset.ID] = true
		kept = append(kept, preset)
	}
	metadata.Presets = append([]SettingsPreset{standard}, kept...)
}

// presetByID は、ID のプリセットを返す。無ければ標準のプリセットを返す。
func presetByID(presets []SettingsPreset, id string) SettingsPreset {
	for _, preset := range presets {
		if preset.ID == id {
			return preset
		}
	}
	for _, preset := range presets {
		if preset.ID == DefaultPresetID {
			return preset
		}
	}
	return SettingsPreset{ID: DefaultPresetID}
}

// SetPresets は、プリセットの一覧をまるごと置き換える。作る・名前を変える・消す・並べ替えるときに
// 使う。base は画面が読んだときの一覧で、そのあとに別の画面やマシンが変えていたら断る。
func (s *Service) SetPresets(base, presets []SettingsPreset) (SaveResult, error) {
	if err := validatePresets(presets); err != nil {
		return SaveResult{}, err
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if !sameSettings(base, stored.Presets) {
		return SaveResult{}, ErrSettingsChanged
	}
	stored.Presets = presets
	return s.commitMetadata(stored, precondition, "settings.presets")
}

// SetPreset は、1つのプリセットの中身を置き換える。base はそのプリセットを画面が読んだときの値で、
// 一覧のほかのプリセットが変わっていても断らない。ID と名前は SetPresets で変える。
func (s *Service) SetPreset(base, preset SettingsPreset) (SaveResult, error) {
	if err := validatePreset(preset); err != nil {
		return SaveResult{}, err
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	index := -1
	for position, candidate := range stored.Presets {
		if candidate.ID == preset.ID {
			index = position
		}
	}
	if index < 0 {
		return SaveResult{}, ErrUnknownPreset
	}
	if base.ID != preset.ID || preset.Name != stored.Presets[index].Name || !sameSettings(base, stored.Presets[index]) {
		return SaveResult{}, ErrSettingsChanged
	}
	if err := s.checkStartDirectoryChange(stored.Presets[index], preset); err != nil {
		return SaveResult{}, err
	}
	stored.Presets[index] = preset
	return s.commitMetadata(stored, precondition, "settings.preset")
}

// sameSettings は、2つの設定が同じ値かを返す。画面が読んで送り返した値と、ディスクから
// 読み直した値は、どちらも JSON から作った値なので、保存する形（JSON）で比べれば揃う。
func sameSettings[T any](first, second T) bool {
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && string(firstJSON) == string(secondJSON)
}
