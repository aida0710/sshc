package application

// TransferSettings は、このマシンで効く SFTP の転送の設定である。プリセット（上限・分割・除外）、
// 全体の設定（自動再接続・完了した転送を消すまで）、マシンの今の状態（処理を止めているか）を
// 合わせたもので、転送キュー（sftp.TransferManager）はこの形で受け取り、保存を頼む。
type TransferSettings struct {
	Preset  FileTransferSettings
	Global  GlobalFileTransferSettings
	Stopped bool
}

// TransferSettings は、このマシンで効く転送の設定を返す。
func (s *Service) TransferSettings() TransferSettings {
	effective := s.EffectiveSettings()
	settings := TransferSettings{Stopped: s.ThisMachineState().TransfersStopped}
	if effective.Preset.FileTransfers != nil {
		settings.Preset = *effective.Preset.FileTransfers
	}
	if effective.Global.FileTransfers != nil {
		settings.Global = *effective.Global.FileTransfers
	}
	return settings
}

// SetTransferSettings は、転送キューで変えた設定を書く。プリセットの部分はこのマシンが使う
// プリセットへ、全体の部分は全体の設定へ、処理を止めているかはこのマシンの今の状態へ書く。
//
// 転送キューの画面と CLI は「このマシンの転送」を変えるので、base を取らず、最後に書いた値を残す。
func (s *Service) SetTransferSettings(settings TransferSettings) error {
	if err := validateFileTransferSettings(&settings.Preset); err != nil {
		return err
	}
	if err := s.writeTransferSettingsToMetadata(settings); err != nil {
		return err
	}
	if s.machineID == "" {
		return nil
	}
	return s.SetTransfersStopped(settings.Stopped)
}

func (s *Service) writeTransferSettingsToMetadata(settings TransferSettings) error {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	inUse := presetByID(stored.Presets, s.ThisMachineSettings().PresetID).ID
	changed := false
	for index := range stored.Presets {
		if stored.Presets[index].ID != inUse {
			continue
		}
		next := storedSection(settings.Preset)
		if !sameSettings(next, stored.Presets[index].FileTransfers) {
			stored.Presets[index].FileTransfers = next
			changed = true
		}
	}
	global := globalSettingsOf(stored)
	if next := storedSection(settings.Global); !sameSettings(next, global.FileTransfers) {
		global.FileTransfers = next
		stored.Settings = storedGlobalSettings(global)
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = s.commitMetadata(stored, precondition, "sftp.transferSettings")
	return err
}
