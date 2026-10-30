package application

// EffectiveSettings は、このマシンで効く設定、つまり全体の設定と、このマシンが使うプリセットと、
// このマシンの設定である。エンジンが当てる設定（セッションの上限、開始ディレクトリ、転送の上限、
// Vault の自動ロックなど）は、すべてここから読む。
type EffectiveSettings struct {
	Global  GlobalSettings
	Preset  SettingsPreset
	Machine MachineSettings
}

// EffectiveSettings は、このマシンで効く設定を返す。metadata を読めなければ、どれも既定の値とする。
func (s *Service) EffectiveSettings() EffectiveSettings {
	machine := s.ThisMachineSettings()
	stored, _, err := s.metadata.Load()
	if err != nil {
		return EffectiveSettings{Preset: SettingsPreset{ID: DefaultPresetID}, Machine: machine}
	}
	return EffectiveSettings{
		Global:  globalSettingsOf(stored),
		Preset:  presetByID(stored.Presets, machine.PresetID),
		Machine: machine,
	}
}

// SettingsDocument は、設定の画面が読むもの全部である。
type SettingsDocument struct {
	// MachineID は、この画面を出しているエンジンのマシンの ID である。
	MachineID string           `json:"machineId"`
	Global    GlobalSettings   `json:"global"`
	Presets   []SettingsPreset `json:"presets"`
	// PresetInUse は、このマシンが使っているプリセットの ID である。選んだプリセットが消えていれば
	// 標準のプリセットになる。
	PresetInUse string    `json:"presetInUse"`
	Machines    []Machine `json:"machines"`
	// State は、このマシンの今の状態である。
	State MachineState `json:"state"`
}

// Settings は、設定の画面が読むもの全部を返す。
func (s *Service) Settings() (SettingsDocument, error) {
	stored, _, err := s.metadata.Load()
	if err != nil {
		return SettingsDocument{}, err
	}
	machines, err := s.Machines()
	if err != nil {
		return SettingsDocument{}, err
	}
	if machines == nil {
		machines = []Machine{}
	}
	return SettingsDocument{
		MachineID:   s.machineID,
		Global:      globalSettingsOf(stored),
		Presets:     stored.Presets,
		PresetInUse: presetByID(stored.Presets, s.ThisMachineSettings().PresetID).ID,
		Machines:    machines,
		State:       s.ThisMachineState(),
	}, nil
}

// OnSettingsChanged は、設定が変わったかもしれないときに呼ぶ関数を足す。エンジンがメモリに持つ
// 設定（転送キューなど）を読み直すために使う。
func (s *Service) OnSettingsChanged(listener func()) {
	s.settingsListeners = append(s.settingsListeners, listener)
}

// SettingsMayHaveChanged は、ファイルの変更（この画面の保存、履歴の復元、同期の取り込み）の
// あとに呼ぶ。Vault の自動ロックと、OnSettingsChanged で足した読み直しを走らせる。
func (s *Service) SettingsMayHaveChanged() {
	s.applyVaultAutoLock()
	for _, listener := range s.settingsListeners {
		listener()
	}
}
