//go:build windows

// Package windowsregistry は、この利用者のためのデスクトップの居場所を覚える。
//
// **HKEY_CURRENT_USER だけを使う。** インストーラは管理者権限を求めない
// per-user のものであり、書き込む先も読む先もこの利用者の枝である。
// HKEY_LOCAL_MACHINE へ書けば、そこを書ける誰かが、この利用者が起こす
// プログラムを決められることになる。
package windowsregistry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"sshc/internal/platform/nativepath"
)

// LauncherKey と LauncherValue は、インストーラと CLI が共有する唯一の場所で
// ある。**綴りを二箇所に持たない。**
const (
	LauncherKey   = `Software\sshc\Desktop`
	LauncherValue = "Executable"
)

// desktopExecutableName は、記録されてよい実体の名前である。
//
// **任意のプログラムを起こす道にしない。** ここに書かれた値は、利用者が
// `sshc` と打っただけで実行される。書ける相手はこの利用者自身に限られるが、
// 書き換えられたことに気づく手立ては要る。
const desktopExecutableName = "sshc.exe"

var (
	// ErrNotRegistered は、まだ誰もデスクトップを登録していないことを表す。
	ErrNotRegistered = errors.New("no sshc desktop application is registered for this user")
	// ErrUnusableExecutable は、記録されている値が起こせるものではないことを表す。
	ErrUnusableExecutable = errors.New("the registered sshc desktop application cannot be started")
)

// ReadDesktopExecutable は、登録された絶対パスを返す。
//
// **返す前に確かめる。** 読んだ文字列をそのまま実行に渡すと、消えた実体も、
// ディレクトリも、別のプログラムも、等しく「起こすもの」になる。
func ReadDesktopExecutable() (string, error) {
	return readDesktopExecutable(LauncherKey)
}

func readDesktopExecutable(key string) (string, error) {
	handle, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("%w: run the sshc installer, or open the sshc application once", ErrNotRegistered)
	}
	defer func() { _ = handle.Close() }()

	value, kind, err := handle.GetStringValue(LauncherValue)
	if err != nil || kind != registry.SZ {
		return "", fmt.Errorf("%w: run the sshc installer, or open the sshc application once", ErrNotRegistered)
	}
	if err := validateDesktopExecutable(value); err != nil {
		return "", err
	}
	return value, nil
}

// validateDesktopExecutable は、記録された値を起こしてよいかを見る。
func validateDesktopExecutable(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("%w: the registered location is empty", ErrUnusableExecutable)
	case !nativepath.Supported(path):
		// 相対パスは作業ディレクトリに意味を与え、\\?\ や \\.\ は
		// このアプリケーションが扱わない名前空間である。
		return fmt.Errorf("%w: %s is not a plain absolute path", ErrUnusableExecutable, path)
	case !strings.EqualFold(filepath.Base(path), desktopExecutableName):
		return fmt.Errorf("%w: %s is not named %s", ErrUnusableExecutable, path, desktopExecutableName)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: %s is not there; reinstall sshc or open it once from its new location",
			ErrUnusableExecutable, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a file", ErrUnusableExecutable, path)
	}
	return nil
}

// RegisterDesktopExecutable は、この利用者のためにデスクトップの居場所を記録する。
//
// **書く前にも確かめる。** 起こせないものを記録しておいて、起こす瞬間に初めて
// 断るより、書けなかったと言う方が早い。
func RegisterDesktopExecutable(path string) error {
	return registerDesktopExecutable(LauncherKey, path)
}

func registerDesktopExecutable(key, path string) error {
	if err := validateDesktopExecutable(path); err != nil {
		return err
	}
	handle, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.SetStringValue(LauncherValue, path)
}

// RemoveDesktopExecutable は、記録が expected と同じときだけ消す。
//
// **他人の記録を消さない。** アンインストーラが呼ぶものなので、二つの版が
// 入っている機械では、いま消そうとしている版のものだけを消す必要がある。
// 別の場所を指しているなら、それは残っている方のインストールのものである。
func RemoveDesktopExecutable(expected string) error {
	return removeDesktopExecutable(LauncherKey, expected)
}

func removeDesktopExecutable(key, expected string) error {
	handle, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		// 記録が無いなら、消すものも無い。
		return nil
	}
	stored, _, err := handle.GetStringValue(LauncherValue)
	if err != nil {
		_ = handle.Close()
		return nil
	}
	// Windows のパスは大文字小文字を区別しない。同じ実体を指す二つの綴りを
	// 別物として扱うと、正しいアンインストーラが自分の記録を消せなくなる。
	if nativepath.Identity(stored) != nativepath.Identity(expected) {
		_ = handle.Close()
		return nil
	}
	err = handle.DeleteValue(LauncherValue)
	_ = handle.Close()
	if err != nil {
		return err
	}
	// 値が消えたなら、この枝そのものも残さない。子を持つ枝は消えないので、
	// 失敗は「他の誰かがまだ使っている」という意味であり、報告しない。
	_ = registry.DeleteKey(registry.CURRENT_USER, key)
	return nil
}
