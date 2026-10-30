package httpserver

import (
	"time"

	"sshc/internal/application"
	sshcSFTP "sshc/internal/sftp"
)

// saveTransferSettings は、engine に適用する前の転送キューの設定を残す。プリセット・全体の設定・
// このマシンの今の状態のどこに書くかは application が決める。
func saveTransferSettings(config *application.Service) func(sshcSFTP.TransferSettings) error {
	return func(settings sshcSFTP.TransferSettings) error {
		return config.SetTransferSettings(storedTransferSettings(settings))
	}
}

// storedTransferSettings は engineTransferSettings の逆で、engine の設定を保存する形にする。
func storedTransferSettings(settings sshcSFTP.TransferSettings) application.TransferSettings {
	return application.TransferSettings{
		Preset: application.FileTransferSettings{
			MaxConcurrent:            settings.MaxConcurrent,
			LargeFileThresholdBytes:  settings.LargeFileThresholdBytes,
			LargeFileParallelism:     settings.LargeFileParallelism,
			LargeFileChunkBytes:      settings.LargeFileChunkBytes,
			SpeedLimitBytesPerSecond: settings.SpeedLimitBytesPerSecond,
			ExcludePatterns:          append([]string(nil), settings.ExcludePatterns...),
		},
		Global: application.GlobalFileTransferSettings{
			ClearCompletedAfterSeconds: int(settings.ClearCompletedAfter / time.Second),
			AutoReconnect:              settings.AutoReconnect,
			MaxReconnectAttempts:       settings.MaxReconnectAttempts,
		},
		Stopped: settings.ProcessingStopped,
	}
}

// engineTransferSettings は、保存されている転送キューの設定を engine の設定の形にする。起動時の
// 復元、設定が変わったときの読み直し、画面・CLI からの更新のどれもがこれを通る。
func engineTransferSettings(stored application.TransferSettings) sshcSFTP.TransferSettings {
	return sshcSFTP.TransferSettings{
		MaxConcurrent:            stored.Preset.MaxConcurrent,
		ClearCompletedAfter:      time.Duration(stored.Global.ClearCompletedAfterSeconds) * time.Second,
		ProcessingStopped:        stored.Stopped,
		LargeFileThresholdBytes:  stored.Preset.LargeFileThresholdBytes,
		LargeFileParallelism:     stored.Preset.LargeFileParallelism,
		LargeFileChunkBytes:      stored.Preset.LargeFileChunkBytes,
		SpeedLimitBytesPerSecond: stored.Preset.SpeedLimitBytesPerSecond,
		AutoReconnect:            stored.Global.AutoReconnect,
		MaxReconnectAttempts:     stored.Global.MaxReconnectAttempts,
		ExcludePatterns:          append([]string(nil), stored.Preset.ExcludePatterns...),
	}
}
