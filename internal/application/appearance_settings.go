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

// AppearanceSettings は、アプリの見た目のうち metadata に置いて同期するものである。
// テーマ（ライト／ダーク）はこのブラウザだけの好みなので、ここには無い。
type AppearanceSettings struct {
	// Accent は、選んでいる項目・ボタン・フォーカスの枠に使う色である。用意した色の名前か、
	// 利用者が決めた色（#rrggbb、小文字）。空は既定の色（画面の既定は lavender）。
	Accent string `json:"accent,omitempty"`
}

// validateAppearanceSettings は、知らないさし色の名前と、色として読めない値を断る。
func validateAppearanceSettings(settings *AppearanceSettings) error {
	if settings == nil || settings.Accent == "" {
		return nil
	}
	if !slices.Contains(appearanceAccents, settings.Accent) && !accentColorPattern.MatchString(settings.Accent) {
		return fmt.Errorf("%w: accent %q", ErrMetadataAppearance, settings.Accent)
	}
	return nil
}

// withoutUnreadableAppearance は、読めないさし色を捨てる。metadata は同期で別のマシンから
// 届くので、ここで捨てないと、画面が metadata をまるごと読めなくなり、設定も接続先の一覧も
// 出せなくなる。捨てた色は既定の色に戻る。
func withoutUnreadableAppearance(metadata *Metadata) {
	if validateAppearanceSettings(metadata.Appearance) != nil {
		metadata.Appearance = nil
	}
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
