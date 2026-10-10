package main

import (
	"context"
	"errors"
	"fmt"
)

func (installer updateInstaller) runHomebrewUpgrade(ctx context.Context, found installation) (string, error) {
	managedPath, err := homebrewManagedExecutable(ctx, found, installer.commands)
	if err != nil {
		return "", err
	}
	if err := installer.commands.Run(ctx, installationProcess{
		name: found.brew, args: []string{"upgrade", "--formula", "--no-ask", homebrewFormula},
		stdout: installer.stdout, stderr: installer.stderr,
	}); err != nil {
		return "", fmt.Errorf("brew upgrade: %w", err)
	}
	return managedPath, nil
}

func (installer updateInstaller) upgradeHomebrewFromWeb(ctx context.Context, found installation) (string, error) {
	managedPath, err := installer.runHomebrewUpgrade(ctx, found)
	if err != nil {
		return "", err
	}
	line, err := installer.commands.Output(ctx, managedPath, "version")
	if err != nil {
		return "", err
	}
	tag, stable := reportedReleaseVersion(line)
	if !stable {
		return "", errors.New("the upgraded Homebrew executable does not report a stable sshc version")
	}
	return tag, nil
}
