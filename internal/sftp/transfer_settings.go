package sftp

import "time"

// TransferSettings は、engine がひとつだけ持つ転送キューの設定である。
//
// 転送は engine の資源であって browser のものではないため、値は engine 側に
// 一つだけ置く。保存先は EnableTransferSettingsPersistence で受け取る。
type TransferSettings struct {
	MaxConcurrent int
	// 0 permits unrestricted payload traffic.
	SpeedLimitBytesPerSecond int64
	AutoReconnect            bool
	MaxReconnectAttempts     int
	// ClearCompletedAfter が 0 なら、完了項目は手動でだけ消える。
	ClearCompletedAfter time.Duration
	// ProcessingStopped の間は待機中の job を新しく開始しない。
	ProcessingStopped       bool
	LargeFileThresholdBytes int64
	LargeFileParallelism    int
	LargeFileChunkBytes     int64
}

// DefaultTransferSettings は、何も保存されていないときの設定である。
func DefaultTransferSettings() TransferSettings {
	return TransferSettings{
		MaxConcurrent:           DefaultTransferConcurrency,
		LargeFileThresholdBytes: DefaultLargeFileThreshold,
		LargeFileParallelism:    DefaultLargeFileParallelism,
		LargeFileChunkBytes:     DefaultLargeFileChunkBytes,
	}
}

// Validate は、範囲外の項目があれば ErrInvalidTransfer を返す。
func (settings TransferSettings) Validate() error {
	if !validMaxConcurrent(settings.MaxConcurrent) || !validClearCompletedAfter(settings.ClearCompletedAfter) ||
		!validLargeFileThreshold(settings.LargeFileThresholdBytes) ||
		!validLargeFileParallelism(settings.LargeFileParallelism) ||
		!validLargeFileChunkBytes(settings.LargeFileChunkBytes) ||
		!validTransferSpeed(settings.SpeedLimitBytesPerSecond) || !validReconnectAttempts(settings.MaxReconnectAttempts) {
		return ErrInvalidTransfer
	}
	return nil
}

func validMaxConcurrent(value int) bool {
	return value >= 1 && value <= MaxTransferConcurrency
}

func validClearCompletedAfter(value time.Duration) bool {
	return value == 0 || (value >= MinClearCompletedAfter && value <= MaxClearCompletedAfter)
}

func validLargeFileThreshold(value int64) bool {
	return value >= MinLargeFileThreshold && value <= MaxLargeFileThreshold
}

func validLargeFileParallelism(value int) bool {
	return value >= 1 && value <= MaxLargeFileParallelism
}

func validLargeFileChunkBytes(value int64) bool {
	return value >= MinLargeFileChunkBytes && value <= MaxLargeFileChunkBytes
}

// EnableTransferSettingsPersistence は、SetTransferSettings が設定を適用する
// 前に save で残すようにする。
func (m *TransferManager) EnableTransferSettingsPersistence(save func(TransferSettings) error) {
	m.settingsMutex.Lock()
	defer m.settingsMutex.Unlock()
	m.saveSettings = save
}

// SetTransferSettings は、画面や CLI が選んだ設定を検査し、保存してから engine
// に適用する。範囲外の項目がひとつでもあれば ErrInvalidTransfer を、保存に
// 失敗すればその error を返し、どちらのときも engine の設定は変えない。
// 保存できなかった設定で動くと、再起動で黙って元に戻るためである。更新は
// 一つずつ通すので、重なった更新でも、最後に保存した設定と engine の設定は
// 同じになる。
func (m *TransferManager) SetTransferSettings(settings TransferSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	m.settingsMutex.Lock()
	defer m.settingsMutex.Unlock()
	if m.saveSettings != nil {
		if err := m.saveSettings(settings); err != nil {
			return err
		}
	}
	m.applyTransferSettings(settings)
	return nil
}

// RestoreTransferSettings は、保存されていた設定で起動する。0 の項目は既定値
// にする。範囲外の項目は、その項目だけを既定値にして名前を rejected に返す。
// 保存値が範囲外になるのは、metadata.json を手で書き換えたときか、新しい
// バージョンが範囲を狭めたときである。そのときも、処理の停止のような範囲内の
// ほかの項目は保存値のまま使う。
func (m *TransferManager) RestoreTransferSettings(stored TransferSettings) (rejected []string) {
	restored, rejected := restorableTransferSettings(stored)
	m.applyTransferSettings(restored)
	return rejected
}

// restorableTransferSettings は、保存値のうち範囲内のものを使い、0 と範囲外の
// 項目を既定値にする。rejected の名前は metadata.json の項目名である。
func restorableTransferSettings(stored TransferSettings) (restored TransferSettings, rejected []string) {
	defaults := DefaultTransferSettings()
	restored.ProcessingStopped = stored.ProcessingStopped
	restored.AutoReconnect = stored.AutoReconnect
	var usableSpeed, usableAttempts bool
	restored.SpeedLimitBytesPerSecond, usableSpeed = storedOrDefault(stored.SpeedLimitBytesPerSecond, int64(0), validTransferSpeed)
	if !usableSpeed {
		rejected = append(rejected, "speedLimitBytesPerSecond")
	}
	restored.MaxReconnectAttempts, usableAttempts = storedOrDefault(stored.MaxReconnectAttempts, 0, validReconnectAttempts)
	if !usableAttempts {
		rejected = append(rejected, "maxReconnectAttempts")
	}
	var usable bool
	if restored.MaxConcurrent, usable = storedOrDefault(stored.MaxConcurrent, defaults.MaxConcurrent, validMaxConcurrent); !usable {
		rejected = append(rejected, "maxConcurrent")
	}
	if restored.ClearCompletedAfter, usable = storedOrDefault(stored.ClearCompletedAfter, defaults.ClearCompletedAfter, validClearCompletedAfter); !usable {
		rejected = append(rejected, "clearCompletedAfterSeconds")
	}
	if restored.LargeFileThresholdBytes, usable = storedOrDefault(stored.LargeFileThresholdBytes, defaults.LargeFileThresholdBytes, validLargeFileThreshold); !usable {
		rejected = append(rejected, "largeFileThresholdBytes")
	}
	if restored.LargeFileParallelism, usable = storedOrDefault(stored.LargeFileParallelism, defaults.LargeFileParallelism, validLargeFileParallelism); !usable {
		rejected = append(rejected, "largeFileParallelism")
	}
	if restored.LargeFileChunkBytes, usable = storedOrDefault(stored.LargeFileChunkBytes, defaults.LargeFileChunkBytes, validLargeFileChunkBytes); !usable {
		rejected = append(rejected, "largeFileChunkBytes")
	}
	return restored, rejected
}

// storedOrDefault は、保存値が 0 か範囲外なら既定値を返す。usable は、保存値が
// 範囲外ではなかったかである。
func storedOrDefault[T int | int64 | time.Duration](stored, fallback T, valid func(T) bool) (value T, usable bool) {
	if stored == 0 {
		return fallback, true
	}
	if !valid(stored) {
		return fallback, false
	}
	return stored, true
}

func (m *TransferManager) applyTransferSettings(settings TransferSettings) {
	m.jobsMutex.Lock()
	defer m.jobsMutex.Unlock()
	m.initializeJobsLocked()
	m.maxConcurrent = settings.MaxConcurrent
	m.clearCompletedAfter = settings.ClearCompletedAfter
	m.processingStopped = settings.ProcessingStopped
	m.speedLimitBytesPerSecond = settings.SpeedLimitBytesPerSecond
	m.autoReconnect = settings.AutoReconnect
	m.maxReconnectAttempts = settings.MaxReconnectAttempts
	m.limiter.setRate(settings.SpeedLimitBytesPerSecond)
	m.largeFileThreshold = settings.LargeFileThresholdBytes
	m.largeFileParallelism = settings.LargeFileParallelism
	m.largeFileChunkBytes = settings.LargeFileChunkBytes
	m.signalSlotLocked()
}
