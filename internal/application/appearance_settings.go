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

// accentColorPattern は、AccentColor の形（#rrggbb、小文字）である。画面は
// <input type="color"> の値をそのまま送り、その値は常にこの形になる。
var accentColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// AppearanceSettings は、アプリの見た目のうち metadata に置いて同期するものである。
// テーマ（ライト／ダーク）はこのブラウザだけの好みなので、ここには無い。
//
// 利用者が決めた色（AccentColor）は、用意した色の名前（Accent）とは別の項目に置く。
// v0.56.0までのsshcは知らない Accent の名前を断り、そう書かれた metadata を保存できなく
// なるが、知らない項目は読み飛ばして Accent の色で出す。同期で古い版と metadata を共有
// しても、古い版は壊れない（古い版が書き直すと AccentColor は消え、Accent の色に戻る）。
type AppearanceSettings struct {
	// Accent は、選んでいる項目・ボタン・フォーカスの枠に使う色の名前である。
	// 空は既定の色（画面の既定は lavender）。AccentColor があるときは、それを知らない
	// 版が使う色になる。
	Accent string `json:"accent,omitempty"`
	// AccentColor は、利用者が決めた色（#rrggbb、小文字）である。あれば Accent より先に使う。
	AccentColor string `json:"accentColor,omitempty"`
}

// validateAppearanceSettings は、知らないさし色の名前と、色として読めない値を断る。
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
