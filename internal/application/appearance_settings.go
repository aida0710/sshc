package application

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// ErrMetadataAppearance は、選べないさし色の名前と、色として読めない値を断る。
var ErrMetadataAppearance = errors.New("appearance settings are out of range")

// appearanceAccents は、用意したさし色の名前である。未読の通知（橙）・接続済み（緑）・
// エラー（赤）と見分けがつく色だけにする。色の値は画面（web/src/index.css）が持つ。
var appearanceAccents = []string{"lavender", "blue", "sky", "pink", "mono"}

// accentColorPattern は、利用者が決めた色の形（#rrggbb、小文字）である。画面は
// <input type="color"> の値をそのまま送り、その値は常にこの形になる。
var accentColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// themeNamePattern はテーマの名前の形である。テーマの一覧は画面（web/src/theme/themes/）が持ち、
// 知らない名前は既定のテーマとして描くので、ここでは形だけを確かめる。
var themeNamePattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// AppearanceSettings は、アプリの見た目（テーマとさし色）である。プリセットに入る。
type AppearanceSettings struct {
	// Theme は、system（OS に合わせる）かテーマの名前である。空は既定のテーマ（画面の既定は dark）。
	Theme string `json:"theme,omitempty"`
	// Accent は、選んでいる項目・ボタン・フォーカスの枠に使う色である。用意した色の名前か、
	// 利用者が決めた色（#rrggbb、小文字）。空は既定の色（画面の既定は lavender）。
	Accent string `json:"accent,omitempty"`
}

// validateAppearanceSettings は、形の合わないテーマの名前、知らないさし色の名前、色として
// 読めない値を断る。
func validateAppearanceSettings(settings *AppearanceSettings) error {
	if settings == nil {
		return nil
	}
	if settings.Theme != "" && !themeNamePattern.MatchString(settings.Theme) {
		return fmt.Errorf("%w: theme %q", ErrMetadataAppearance, settings.Theme)
	}
	if settings.Accent != "" && !slices.Contains(appearanceAccents, settings.Accent) && !accentColorPattern.MatchString(settings.Accent) {
		return fmt.Errorf("%w: accent %q", ErrMetadataAppearance, settings.Accent)
	}
	return nil
}
