package application

import (
	"errors"
	"fmt"
	"slices"
)

// ErrMetadataAppearance は、選べないさし色の名前を断る。
var ErrMetadataAppearance = errors.New("appearance settings are out of range")

// appearanceAccents は、選べるさし色の名前である。未読の通知（橙）・接続済み（緑）・
// エラー（赤）と見分けがつく色だけにする。色の値は画面（web/src/index.css）が持つ。
var appearanceAccents = []string{"lavender", "blue", "sky", "pink", "mono"}

// AppearanceSettings は、アプリの見た目のうち metadata に置いて同期するものである。
// テーマ（ライト／ダーク）はこのブラウザだけの好みなので、ここには無い。
type AppearanceSettings struct {
	// Accent は、選んでいる項目・ボタン・フォーカスの枠に使う色の名前である。
	// 空は既定の色（画面の既定は lavender）。
	Accent string `json:"accent,omitempty"`
}

// validateAppearanceSettings は、知らないさし色の名前を断る。
func validateAppearanceSettings(settings *AppearanceSettings) error {
	if settings == nil || settings.Accent == "" {
		return nil
	}
	if !slices.Contains(appearanceAccents, settings.Accent) {
		return fmt.Errorf("%w: accent %q", ErrMetadataAppearance, settings.Accent)
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
