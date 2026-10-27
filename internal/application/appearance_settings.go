package application

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// ErrMetadataAppearance は、選べないさし色の名前と、色として読めない値を断る。
var ErrMetadataAppearance = errors.New("appearance settings are out of range")

// appearanceAccents は、選べるさし色の名前である。用意した色は、未読の通知（橙）・
// 接続済み（緑）・エラー（赤）と見分けがつく色だけにする。色の値は画面
// （web/src/index.css）が持つ。customAccent は、利用者が決めた AccentColor の色を使う。
var appearanceAccents = []string{"lavender", "blue", "sky", "pink", "mono", customAccent}

const customAccent = "custom"

// accentColorPattern は、AccentColor の形（#rrggbb、小文字）である。画面は
// <input type="color"> の値をそのまま送り、その値は常にこの形になる。
var accentColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// AppearanceSettings は、アプリの見た目のうち metadata に置いて同期するものである。
// テーマ（ライト／ダーク）はこのブラウザだけの好みなので、ここには無い。
type AppearanceSettings struct {
	// Accent は、選んでいる項目・ボタン・フォーカスの枠に使う色の名前である。
	// 空は既定の色（画面の既定は lavender）。
	Accent string `json:"accent,omitempty"`
	// AccentColor は、Accent が custom のときの色である。用意した色を選んでいる間も
	// 残し、custom に戻したときに同じ色を出す。
	AccentColor string `json:"accentColor,omitempty"`
}

// validateAppearanceSettings は、知らないさし色の名前と、色の無い custom を断る。
func validateAppearanceSettings(settings *AppearanceSettings) error {
	if settings == nil {
		return nil
	}
	if settings.Accent != "" && !slices.Contains(appearanceAccents, settings.Accent) {
		return fmt.Errorf("%w: accent %q", ErrMetadataAppearance, settings.Accent)
	}
	if settings.AccentColor != "" && !accentColorPattern.MatchString(settings.AccentColor) {
		return fmt.Errorf("%w: accent colour %q", ErrMetadataAppearance, settings.AccentColor)
	}
	if settings.Accent == customAccent && settings.AccentColor == "" {
		return fmt.Errorf("%w: a custom accent needs its colour", ErrMetadataAppearance)
	}
	return nil
}

// SetAppearanceSettings は、節をまるごと置き換える。
func (s *Service) SetAppearanceSettings(settings AppearanceSettings) (SaveResult, error) {
	if err := validateAppearanceSettings(&settings); err != nil {
		return SaveResult{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if settings == (AppearanceSettings{}) {
		// 何も選ばれていないなら節ごと消す。空の節を残さない。
		stored.Appearance = nil
	} else {
		stored.Appearance = &settings
	}
	return s.commitMetadata(stored, precondition, "appearance.settings")
}
