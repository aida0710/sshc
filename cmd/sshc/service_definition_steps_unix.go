//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sshc/internal/storage"
)

// serviceDefinitionSteps は、定義ファイル（systemd の unit、launchd の plist）を書いて
// サービスを登録する手順と、外す手順の骨組みである。systemd と launchd で違うのは、
// 止め方と始め方だけなので、そこだけを関数で受け取る。
type serviceDefinitionSteps struct {
	definition serviceDefinitionFile
	lock       func() (func() error, error)
	// temporaryPrefix は、定義を書き換えるときの一時ファイルの名前の頭である。
	temporaryPrefix string
	// stopForInstall は、定義を書き換える前に動いているサービスを止める。nil なら止めない。
	stopForInstall func(context.Context) error
	// start は、書いた定義でサービスを始める。
	start     func(context.Context) error
	waitReady func(context.Context) error
	// stopForRemoval は、定義を消す前にサービスを止めて登録を外す。
	stopForRemoval func(context.Context) error
	// afterRemoval は、定義を消したあとに呼ぶ。nil なら何もしない。
	afterRemoval func(context.Context) error
}

// install は、executable を登録した定義を書いてサービスを始め、準備ができるまで待つ。
// 手書きの定義は書き換えない。止めてから書くまでのあいだに定義が変わっていたら、書かずに断る。
func (steps serviceDefinitionSteps) install(ctx context.Context, executable string) (result error) {
	release, err := acquireServiceOperationLock(steps.lock)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := steps.definition.readSnapshot()
	if err != nil {
		return err
	}
	if snapshot.state == serviceUnmanaged {
		return errUnmanagedServiceUnit
	}
	contents, err := steps.definition.render(executable)
	if err != nil {
		return err
	}
	if steps.stopForInstall != nil {
		if err := steps.stopForInstall(ctx); err != nil {
			return err
		}
	}
	path := steps.definition.path
	if err := steps.definition.files.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := steps.definition.ensureUnchanged(snapshot); err != nil {
		return err
	}
	if err := storage.WriteAtomicFile(steps.definition.files, path, steps.temporaryPrefix, 0o600, []byte(contents)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := steps.start(ctx); err != nil {
		return err
	}
	if err := steps.waitReady(ctx); err != nil {
		return fmt.Errorf("service did not become ready: %w", err)
	}
	return steps.definition.ensureStillMatches(executable, "starting")
}

// disable は、サービスを止めて定義を消す。定義が無ければ何もせず false を返す。
func (steps serviceDefinitionSteps) disable(ctx context.Context) (removed bool, result error) {
	release, err := acquireServiceOperationLock(steps.lock)
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := steps.definition.readSnapshot()
	if err != nil {
		return false, err
	}
	if snapshot.state == serviceAbsent {
		return false, nil
	}
	if snapshot.state == serviceUnmanaged {
		return false, errUnmanagedServiceUnit
	}
	if err := steps.stopForRemoval(ctx); err != nil {
		return false, err
	}
	if err := steps.definition.ensureUnchanged(snapshot); err != nil {
		return false, err
	}
	path := steps.definition.path
	if err := steps.definition.files.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}
	if steps.afterRemoval != nil {
		if err := steps.afterRemoval(ctx); err != nil {
			return false, err
		}
	}
	return true, nil
}
