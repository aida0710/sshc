//go:build windows

package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"golang.org/x/sys/windows"

	"sshc/internal/app"
	"sshc/internal/platform/windowsacl"
)

func newPlatformServiceManager(home string) (engineServiceManager, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("home directory is not absolute")
	}
	user, err := windowsacl.CurrentUserSID()
	if err != nil {
		return nil, fmt.Errorf("read current user SID: %w", err)
	}
	// The Windows API resolves %SystemRoot%\System32 without trusting PATH or USERDOMAIN.
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	stateDir, err := app.StateDir(home)
	if err != nil {
		return nil, err
	}
	engine := taskEngine{stateDir: stateDir, client: &http.Client{Timeout: connectTimeout}}
	return &taskServiceManager{
		sid:              user.String(),
		runner:           osServiceCommandRunner{path: filepath.Join(systemDirectory, "schtasks.exe")},
		readTask:         readWindowsServiceTask,
		engineRunning:    func() (bool, error) { return isEngineRunning(stateDir) },
		engineResponding: engine.probe,
		stopEngine:       engine.stop,
		waitReady:        engine.wait,
		lock:             serviceOperationLock(home),
	}, nil
}
