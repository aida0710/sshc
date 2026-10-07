package httpserver

import (
	"time"

	"sshc/internal/application"
	sshcSFTP "sshc/internal/sftp"
)

// saveTransferSettings は、engine に適用する前の転送キューの設定を
// metadata.json へ残す。
func saveTransferSettings(config *application.Service) func(sshcSFTP.TransferSettings) error {
	return func(settings sshcSFTP.TransferSettings) error {
		_, err := config.SetFileTransferSettings(storedTransferSettings(settings))
		return err
	}
}

// storedTransferSettings は engineTransferSettings の逆で、engine の設定を
// metadata.json に保存する形にする。
func storedTransferSettings(settings sshcSFTP.TransferSettings) application.FileTransferSettings {
	return application.FileTransferSettings{
		MaxConcurrent:              settings.MaxConcurrent,
		ClearCompletedAfterSeconds: int(settings.ClearCompletedAfter / time.Second),
		ProcessingStopped:          settings.ProcessingStopped,
		LargeFileThresholdBytes:    settings.LargeFileThresholdBytes,
		LargeFileParallelism:       settings.LargeFileParallelism,
		LargeFileChunkBytes:        settings.LargeFileChunkBytes,
		SpeedLimitBytesPerSecond:   settings.SpeedLimitBytesPerSecond,
		AutoReconnect:              settings.AutoReconnect,
		MaxReconnectAttempts:       settings.MaxReconnectAttempts,
	}
}

// engineTransferSettings は、metadata.json に保存する転送キューの設定を、
// engine の設定の形にする。起動時の復元と画面・CLI からの更新の両方がこれを
// 通る。
func engineTransferSettings(stored application.FileTransferSettings) sshcSFTP.TransferSettings {
	return sshcSFTP.TransferSettings{
		MaxConcurrent:            stored.MaxConcurrent,
		ClearCompletedAfter:      time.Duration(stored.ClearCompletedAfterSeconds) * time.Second,
		ProcessingStopped:        stored.ProcessingStopped,
		LargeFileThresholdBytes:  stored.LargeFileThresholdBytes,
		LargeFileParallelism:     stored.LargeFileParallelism,
		LargeFileChunkBytes:      stored.LargeFileChunkBytes,
		SpeedLimitBytesPerSecond: stored.SpeedLimitBytesPerSecond,
		AutoReconnect:            stored.AutoReconnect,
		MaxReconnectAttempts:     stored.MaxReconnectAttempts,
	}
}
