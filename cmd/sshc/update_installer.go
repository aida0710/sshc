package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/releasecheck"
)

func (installer updateInstaller) runTaggedInstaller(ctx context.Context, found installation, tag string) error {
	if stable, ok := releasecheck.StableTag(tag); !ok || stable != tag {
		return fmt.Errorf("refuse installer tag %q", tag)
	}
	name := found.manager.installerName()
	if found.manager == managerPowerShell {
		matched, err := installerReceiptMatches(found.executable, name)
		if err != nil {
			return err
		}
		if !matched {
			return errors.New("the executable is not managed by install.ps1")
		}
	}
	script, err := installer.downloadTaggedInstaller(ctx, tag, name)
	if err != nil {
		return err
	}
	if err := installer.runInstallerScript(ctx, installerScript{name: name, contents: script, environment: installer.environment(found, tag)}); err != nil {
		return err
	}
	matched, err := installerReceiptMatches(found.executable, name)
	if err != nil {
		return fmt.Errorf("verify the updated install receipt: %w", err)
	}
	if !matched {
		return fmt.Errorf("%s completed without a valid install receipt", name)
	}
	if err := installer.verifyReportedVersion(ctx, found.executable, tag); err != nil {
		return fmt.Errorf("verify the updated executable: %w", err)
	}
	return nil
}

// downloadTaggedInstaller bounds the script size and pins its source to the release tag.
func (installer updateInstaller) downloadTaggedInstaller(ctx context.Context, tag, name string) ([]byte, error) {
	url := "https://" + taggedInstallerHost + "/" + installRepository + "/" + tag + "/" + name
	script, err := downloadInstallerFile(ctx, installer.client, url)
	if err != nil || name != "install.ps1" {
		return script, err
	}
	// Unlike install.sh, install.ps1 is also a checksummed Release asset. Bind
	// the tagged source to that published asset before executing any PowerShell.
	client := *installer.client
	client.CheckRedirect = allowReleaseAssetRedirect
	checksums, err := downloadInstallerFile(ctx, &client, "https://github.com/"+installRepository+"/releases/download/"+tag+"/checksums.txt")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(script)
	want := hex.EncodeToString(digest[:])
	entries := 0
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		entries++
		if !strings.EqualFold(fields[0], want) {
			return nil, errors.New("install.ps1 does not match its published checksum")
		}
	}
	if entries != 1 {
		return nil, errors.New("checksums.txt must list install.ps1 exactly once")
	}
	return script, nil
}

func allowReleaseAssetRedirect(request *http.Request, previous []*http.Request) error {
	// GitHub serves release downloads from its dedicated asset host.
	// Keep net/http's default ten-redirect bound when overriding CheckRedirect.
	const maximumReleaseRedirects = 10
	host := request.URL.Host
	if len(previous) >= maximumReleaseRedirects || request.URL.Scheme != "https" ||
		(host != "github.com" && host != "release-assets.githubusercontent.com") {
		return errors.New("release asset redirected outside the allowed GitHub HTTPS hosts")
	}
	return nil
}

func downloadInstallerFile(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download the tagged installer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download the tagged installer: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxInstallerSize {
		return nil, errors.New("the tagged installer is unexpectedly large")
	}
	script, err := io.ReadAll(io.LimitReader(response.Body, maxInstallerSize+1))
	if err != nil {
		return nil, fmt.Errorf("read the tagged installer: %w", err)
	}
	if len(script) > maxInstallerSize {
		return nil, errors.New("the tagged installer is unexpectedly large")
	}
	return script, nil
}

// environment fixes the release and destination and assigns engine restart ownership.
func (installer updateInstaller) environment(found installation, tag string) []string {
	caller := installer.caller
	if caller == "" {
		caller = "update"
	}
	return replaceEnvironment(os.Environ(), map[string]string{
		"SSHC_VERSION":        tag,
		"SSHC_INSTALL_DIR":    filepath.Dir(found.executable),
		"SSHC_INSTALL_CALLER": caller,
		// An inherited test fixture must never supply bytes for an automatic update.
		"SSHC_INSTALL_TESTING":  "",
		"SSHC_TEST_RELEASE_DIR": "",
	})
}

type installerScript struct {
	name        string
	contents    []byte
	environment []string
}

// The Windows installer renames the running image before publishing its replacement.
// Both this CLI and the web engine can keep using that image until they exit.
func (installer updateInstaller) runInstallerScript(ctx context.Context, script installerScript) error {
	temporary, err := os.CreateTemp("", "sshc-install-*"+filepath.Ext(script.name))
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(script.contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	shell, arguments := "sh", []string{temporaryPath}
	if script.name == "install.ps1" {
		shell = "powershell.exe"
		arguments = []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", temporaryPath}
	}
	if err := installer.commands.Run(ctx, installationProcess{
		name: shell, args: arguments, environment: script.environment,
		stdout: installer.stdout, stderr: installer.stderr,
	}); err != nil {
		return fmt.Errorf("%s: %w", script.name, err)
	}
	return nil
}
