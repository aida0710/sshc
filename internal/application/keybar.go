package application

import (
	"errors"

	"sshc/internal/validate"
)

// KeyBarKey は、スマホのキーバーの1つのキーである。用意したキー（Key）か、自作のキー（Label と
// Text）のどちらか一方を持つ。
type KeyBarKey struct {
	// Key は、用意したキーの名前（validate.KeyBarKeys）である。
	Key string `json:"key,omitempty"`
	// Label は、自作のキーに出す文字である。
	Label string `json:"label,omitempty"`
	// Text は、自作のキーが送る内容である。打ったのと同じように送る。
	Text string `json:"text,omitempty"`
	// Enter は、自作のキーが Text を送ったあと Enter を押すかである。
	Enter bool `json:"enter,omitempty"`
}

// KeyBarPreset は、名前の付いたキーバーの並びである。どの並びを使うかは、設定のプリセットが
// 選ぶ（InterfaceSettings.KeyBarPreset）。
type KeyBarPreset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Keys は、いつも出す段のキーである。
	Keys []KeyBarKey `json:"keys"`
	// MoreKeys は、「…」を押すと出るキーである。空なら「…」を出さない。
	MoreKeys []KeyBarKey `json:"moreKeys"`
}

var ErrKeyBarPresets = errors.New("invalid key bar presets")
var ErrKeyBarConflict = errors.New("key bar presets changed; reload before saving")

func validateKeyBarPresets(presets []KeyBarPreset) error {
	if len(presets) > validate.MaxKeyBarPresets {
		return ErrKeyBarPresets
	}
	ids := map[string]bool{}
	for _, p := range presets {
		if !validKeyBarPreset(p) || ids[p.ID] {
			return ErrKeyBarPresets
		}
		ids[p.ID] = true
	}
	return nil
}

// validKeyBarPreset は、p の名前と並びが決まりどおりかを返す。用意したキーは、2つの段を通して
// 1回ずつしか置けない。自作のキーは同じものを何度置いてもよい。
func validKeyBarPreset(p KeyBarPreset) bool {
	if !validPresetLibraryEntry(p.ID, p.Name, validate.MaxKeyBarPresetNameLength) ||
		p.Keys == nil || p.MoreKeys == nil || len(p.Keys) > validate.MaxKeyBarKeys || len(p.MoreKeys) > validate.MaxKeyBarMoreKeys {
		return false
	}
	placed := map[string]bool{}
	for _, key := range append(append([]KeyBarKey(nil), p.Keys...), p.MoreKeys...) {
		if !validKeyBarKey(key) || (key.Key != "" && placed[key.Key]) {
			return false
		}
		placed[key.Key] = key.Key != ""
	}
	return true
}

func validKeyBarKey(key KeyBarKey) bool {
	if key.Key != "" {
		return validate.IsKeyBarKey(key.Key) && key.Label == "" && key.Text == "" && !key.Enter
	}
	return validate.KeyBarLabel(key.Label) == nil && validate.KeyBarText(key.Text) == nil
}

// withoutUnreadableKeyBarPresets は、このビルドが使えるキーバーのプリセットだけを残す。後のビルドが
// 足した用意したキーは、そのキーだけを読み飛ばし、利用者が作った並びは残す。それでも決まりに合わない
// プリセットは捨てる。残すと、metadata を保存するたびに検証で断られる。
func withoutUnreadableKeyBarPresets(metadata *Metadata) {
	kept := metadata.KeyBarPresets[:0]
	ids := map[string]bool{}
	for _, p := range metadata.KeyBarPresets {
		p.Keys = withoutUnknownKeyBarKeys(p.Keys)
		p.MoreKeys = withoutUnknownKeyBarKeys(p.MoreKeys)
		if len(kept) >= validate.MaxKeyBarPresets || !validKeyBarPreset(p) || ids[p.ID] {
			continue
		}
		ids[p.ID] = true
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		kept = nil
	}
	metadata.KeyBarPresets = kept
}

func withoutUnknownKeyBarKeys(keys []KeyBarKey) []KeyBarKey {
	known := []KeyBarKey{}
	for _, key := range keys {
		if key.Key == "" || validate.IsKeyBarKey(key.Key) {
			known = append(known, key)
		}
	}
	return known
}

// SetKeyBarPresets は、キーバーのプリセットの一覧を presets に置き換える。base は画面が読んだときの
// 一覧で、ほかの端末が先に変えていれば ErrKeyBarConflict を返す。
func (s *Service) SetKeyBarPresets(base, presets []KeyBarPreset) (SaveResult, error) {
	if err := validateKeyBarPresets(presets); err != nil {
		return SaveResult{}, err
	}
	return replacePresetLibrary(s, "keybar.presets", base, presets, func(stored *Metadata) *[]KeyBarPreset { return &stored.KeyBarPresets }, ErrKeyBarConflict)
}
