//go:build darwin

package macos

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework Foundation -framework Security -framework LocalAuthentication
#include <stdlib.h>
#include "biometric_darwin.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"sshc/internal/secret"
)

// errSecUserCanceled は、人がプロンプトを閉じたときに Keychain が返す番号である。
// errSecAuthFailed は、指が通らなかったときのものだ。**どちらも失敗ではない。**
const (
	errSecUserCanceled = -128
	errSecAuthFailed   = -25293
	errSecItemNotFound = -25300
)

// Biometric は、macOS の Keychain を番人にする。
//
// **engine 自身がこれを叩ける。** darwin のビルドは既に cgo を有効にしており、
// プロンプトを出すのは SecItemCopyMatching なので、窓を持っている必要が無い。
type Biometric struct{}

// available は、測った答えを一度だけ覚える。**Keychain を毎回突かない。**
var available = sync.OnceValue(func() bool {
	if C.sshc_biometric_available() != 1 {
		return false
	}
	// **能力を仮定せず、実際に預けてみる。**
	//
	// 生体で守られた項目は data protection keychain のものであり、そこは
	// `keychain-access-groups` の entitlement を持つ**署名された**実行体にしか
	// 開かない。署名の無いこの束では `SecItemAdd` が errSecMissingEntitlement
	// (-34018) を返す——実測した。ad-hoc 署名で entitlement を主張すると、
	// 今度は AMFI がプロセスごと殺す。これも実測した。
	//
	// つまりこの機能が使えるかどうかは、**この実行体がどう署名されているか**で
	// 決まる。尋ねる相手は Keychain 自身であって、こちらの願望ではない。配布用の
	// 署名を手に入れた日、ここは何も変えずに true になる。
	probe := []byte("sshc-probe")
	if status := C.sshc_biometric_keep(unsafe.Pointer(&probe[0]), C.int(len(probe))); status != 0 {
		return false
	}
	C.sshc_biometric_forget()
	return true
})

// Available は、この機械が生体で本人を確かめられ、**かつ預かってもらえる**かを
// 答える。
func (Biometric) Available() bool { return available() }

// Keep は預ける。以後、取り出すには登録済みの指が要る。
func (Biometric) Keep(kept []byte) error {
	if len(kept) == 0 {
		return secret.ErrEmptySecret
	}
	if status := C.sshc_biometric_keep(unsafe.Pointer(&kept[0]), C.int(len(kept))); status != 0 {
		return fmt.Errorf("%w: keychain said %d", secret.ErrNoGuardian, int(status))
	}
	return nil
}

// Reveal は、指を通してから返す。
func (Biometric) Reveal() ([]byte, error) {
	var out unsafe.Pointer
	var length C.int
	switch status := C.sshc_biometric_reveal(&out, &length); {
	case status == 0:
	case status == errSecUserCanceled || status == errSecAuthFailed:
		return nil, secret.ErrRefused
	case status == errSecItemNotFound:
		return nil, secret.ErrNoBiometric
	default:
		return nil, secret.ErrRefused
	}
	defer C.free(out)
	return C.GoBytes(out, length), nil
}

// Forget は預かりを解く。預かっていなくても成功する。
func (Biometric) Forget() error {
	C.sshc_biometric_forget()
	return nil
}
