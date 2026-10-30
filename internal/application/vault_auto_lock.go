package application

import (
	"errors"
	"fmt"
	"time"

	"sshc/internal/secret"
)

// ErrVaultAutoLock は、自動ロックの選択が範囲の外であることを表す。
var ErrVaultAutoLock = errors.New("vault auto lock is invalid")

// VaultAutoLock は、導出済みのVault鍵をmemoryから破棄する条件である。プリセットに入り、
// マシンは使っているプリセットの値に従う。restartは自動タイマーを無効にするが、手動ロックと
// engine終了は引き続き鍵を破棄する。
type VaultAutoLock struct {
	Mode  string `json:"mode"`
	Value int    `json:"value,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

const (
	VaultAutoLockIdle    = "idle"
	VaultAutoLockRestart = "restart"
	VaultAutoLockMinutes = "minutes"
	VaultAutoLockHours   = "hours"
)

// VaultAutoLock の Value（分または時間）の範囲。設定画面は 3 桁までの数で入力させる。
const (
	MinVaultAutoLockValue = 1
	MaxVaultAutoLockValue = 999
)

func validateVaultAutoLock(chosen *VaultAutoLock) error {
	if chosen == nil {
		return nil
	}
	switch chosen.Mode {
	case VaultAutoLockRestart:
		if chosen.Value != 0 || chosen.Unit != "" {
			return fmt.Errorf("%w: restart-only auto lock has a duration", ErrVaultAutoLock)
		}
	case VaultAutoLockIdle:
		if chosen.Value < MinVaultAutoLockValue || chosen.Value > MaxVaultAutoLockValue ||
			(chosen.Unit != VaultAutoLockMinutes && chosen.Unit != VaultAutoLockHours) {
			return fmt.Errorf("%w: idle auto lock duration", ErrVaultAutoLock)
		}
	default:
		return fmt.Errorf("%w: mode %q", ErrVaultAutoLock, chosen.Mode)
	}
	return nil
}

// vaultIdleTimeout は保存済みの選択を実行時の時間へ変換する。
// 未設定はfallbackを使い、restartは自動ロックなしを表す0を返す。
func vaultIdleTimeout(chosen *VaultAutoLock, fallback time.Duration) time.Duration {
	if chosen == nil {
		return fallback
	}
	if chosen.Mode == VaultAutoLockRestart {
		return 0
	}
	unit := time.Minute
	if chosen.Unit == VaultAutoLockHours {
		unit = time.Hour
	}
	return time.Duration(chosen.Value) * unit
}

// applyVaultAutoLock は、このマシンが使っているプリセットの自動ロックの時間を Vault の時計へ
// 移す。設定が変わりうる操作（保存、履歴の復元、同期の取り込み、使うプリセットの切り替え）の
// あとに呼ぶ（settingsChanged）。
func (s *Service) applyVaultAutoLock() {
	if s.vault == nil {
		return
	}
	s.vault.SetIdleTimeout(vaultIdleTimeout(s.EffectiveSettings().Preset.VaultAutoLock, secret.IdleTimeout))
}
