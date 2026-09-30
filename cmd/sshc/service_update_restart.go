//go:build linux || darwin

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// restartManagedServiceAfterUpdate は、sshc service install で登録した service が今回の
// 実行ファイルを指していて、動いていれば、新しい版で再起動する。
//
// 定義ファイルが今回の実行ファイルを指すかは、ツール（systemctl、launchctl）を探す前に
// 確かめる。service を登録していないマシンでは、ツールが無くても sshc update を妨げない。
// sshc が書いた定義が以前の版の形のままなら、再起動せずに errOutdatedServiceDefinition を返す。
func restartManagedServiceAfterUpdate(ctx context.Context, executable string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve home directory: %w", err)
	}
	if !filepath.IsAbs(home) {
		return false, nil
	}
	manager := newServiceManagerWithoutTool(filepath.Clean(home))
	definition := manager.definitionFile()
	matches, err := definition.matches(executable)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, outdatedDefinitionError(definition)
	}
	if err := manager.resolveTool(); err != nil {
		return false, err
	}
	return manager.RestartIfActive(ctx, executable)
}
