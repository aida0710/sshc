package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sshc/internal/selfupdate"
)

const (
	latestReleaseAPI  = "https://api.github.com/repos/aida0710/sshc/releases/latest"
	homebrewFormula   = "aida0710/tap/sshc"
	installRepository = "aida0710/sshc"
	receiptFileName   = ".sshc-install-receipt.json"
	maxInstallerSize  = 1 << 20
)

type installManager uint8

const (
	managerUnknown installManager = iota
	managerHomebrew
	managerShell
)

type installation struct {
	manager    installManager
	executable string
	brew       string
}

type updateDependencies struct {
	executable     func() (string, error)
	detect         func(string) (installation, error)
	latest         func(context.Context) (selfupdate.Release, error)
	install        func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error
	restartService func(context.Context) (bool, error)
}

func defaultUpdateDependencies() updateDependencies {
	checker := &selfupdate.Checker{
		API:  latestReleaseAPI,
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
	installerClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if request.URL.Scheme != "https" || request.URL.Hostname() != "raw.githubusercontent.com" {
				return errors.New("the tagged installer redirected outside raw.githubusercontent.com")
			}
			return nil
		},
	}
	commands := osUpdateCommands{}
	return updateDependencies{
		executable: os.Executable,
		detect:     detectInstallation,
		latest:     checker.Latest,
		install: func(ctx context.Context, found installation, release selfupdate.Release, stdout, stderr io.Writer) error {
			return installUpdate(ctx, found, release, installerClient, commands, stdout, stderr)
		},
		restartService: restartManagedServiceAfterUpdate,
	}
}

func runUpdate(ctx context.Context, current string, stdout, stderr io.Writer, dependencies updateDependencies) int {
	executable, err := dependencies.executable()
	if err != nil {
		fmt.Fprintf(stderr, "sshc: find this executable: %v\n", err)
		return 1
	}
	found, err := dependencies.detect(executable)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: inspect this installation: %v\n", err)
		return 1
	}
	if found.manager == managerUnknown {
		fmt.Fprintf(stderr, "sshc: %s is not managed by Homebrew or sshc's install.sh, so it cannot be updated automatically\n", executable)
		fmt.Fprintln(stderr, "sshc: update it with the method that installed it")
		return 1
	}

	latest, err := dependencies.latest(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		if errors.Is(err, selfupdate.ErrNoRelease) {
			fmt.Fprintln(stderr, "sshc: no release is available")
		} else {
			fmt.Fprintf(stderr, "sshc: check the latest release: %v\n", err)
		}
		return 1
	}
	tag, ok := selfupdate.StableTag(latest.Version)
	if !ok {
		fmt.Fprintf(stderr, "sshc: the latest release has an invalid version %q\n", latest.Version)
		return 1
	}
	latest.Version = tag
	if !selfupdate.Newer(current, tag) {
		fmt.Fprintf(stdout, "sshc: %s is already the latest release\n", current)
		return 0
	}

	fmt.Fprintf(stdout, "sshc: updating %s to %s\n", current, tag)
	if err := dependencies.install(ctx, found, latest, stdout, stderr); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return 130
		}
		fmt.Fprintf(stderr, "sshc: update failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sshc: updated to %s\n", tag)
	if dependencies.restartService != nil {
		restarted, err := dependencies.restartService(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return 130
			}
			fmt.Fprintf(stderr, "sshc: update succeeded, but restart the managed service: %v\n", err)
			return 1
		}
		if restarted {
			fmt.Fprintln(stdout, "sshc: managed service restarted; vault is locked")
			fmt.Fprintln(stdout, "sshc: run `sshc vault unlock` from an interactive terminal")
			return 0
		}
	}
	fmt.Fprintln(stdout, "sshc: restart any running `sshc engine` to use the new version")
	return 0
}

type installReceipt struct {
	SchemaVersion int    `json:"schemaVersion"`
	Manager       string `json:"manager"`
	Repository    string `json:"repository"`
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
}

func detectInstallation(executable string) (installation, error) {
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return installation{}, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return installation{}, err
	}
	if runtime.GOOS != "windows" {
		if brew, ok := homebrewForExecutable(resolved); ok {
			return installation{manager: managerHomebrew, executable: resolved, brew: brew}, nil
		}
		if filepath.Base(resolved) != "sshc" {
			return installation{manager: managerUnknown, executable: resolved}, nil
		}
		matched, receiptErr := shellReceiptMatches(resolved)
		if receiptErr != nil {
			return installation{}, receiptErr
		}
		if matched {
			return installation{manager: managerShell, executable: resolved}, nil
		}
	}
	return installation{manager: managerUnknown, executable: resolved}, nil
}

func homebrewForExecutable(executable string) (string, bool) {
	clean := filepath.Clean(executable)
	parts := strings.Split(clean, string(filepath.Separator))
	for index := 0; index+3 < len(parts); index++ {
		if parts[index] != "Cellar" || parts[index+1] != "sshc" {
			continue
		}
		prefix := strings.Join(parts[:index], string(filepath.Separator))
		if filepath.IsAbs(clean) && prefix == "" {
			prefix = string(filepath.Separator)
		}
		brew := filepath.Join(prefix, "bin", "brew")
		if info, err := os.Stat(brew); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return brew, true
		}
	}
	return "", false
}

func shellReceiptMatches(executable string) (bool, error) {
	path := filepath.Join(filepath.Dir(executable), receiptFileName)
	linkInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !linkInfo.Mode().IsRegular() || linkInfo.Size() > 4096 {
		return false, fmt.Errorf("%s is not a valid regular install receipt", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return false, fmt.Errorf("%s is not a valid install receipt", path)
	}
	var receipt installReceipt
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("%s contains trailing data", path)
	}
	if receipt.SchemaVersion != 1 || receipt.Manager != "install.sh" || receipt.Repository != installRepository {
		return false, fmt.Errorf("%s does not describe an sshc install.sh installation", path)
	}
	if _, ok := selfupdate.StableTag(receipt.Version); !ok || len(receipt.SHA256) != sha256.Size*2 {
		return false, fmt.Errorf("%s contains invalid release metadata", path)
	}
	if _, err := hex.DecodeString(receipt.SHA256); err != nil {
		return false, fmt.Errorf("%s contains an invalid SHA-256 digest", path)
	}
	digest, err := fileSHA256(executable)
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(receipt.SHA256, digest) {
		return false, fmt.Errorf("%s does not match the installed executable", path)
	}
	return true, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

type updateCommands interface {
	Output(context.Context, string, ...string) ([]byte, error)
	Run(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error
}

type osUpdateCommands struct{}

func (osUpdateCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	configureUpdateCommand(command)
	return command.Output()
}

func (osUpdateCommands) Run(ctx context.Context, name string, args []string, environment []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, name, args...)
	configureUpdateCommand(command)
	if environment != nil {
		command.Env = environment
	}
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	return command.Run()
}

func installUpdate(ctx context.Context, found installation, release selfupdate.Release, client *http.Client, commands updateCommands, stdout, stderr io.Writer) error {
	switch found.manager {
	case managerHomebrew:
		return upgradeHomebrew(ctx, found, release.Version, commands, stdout, stderr)
	case managerShell:
		return runTaggedInstaller(ctx, found, release.Version, client, commands, stdout, stderr)
	default:
		return errors.New("unsupported installation manager")
	}
}

func upgradeHomebrew(ctx context.Context, found installation, tag string, commands updateCommands, stdout, stderr io.Writer) error {
	managedPath, err := homebrewManagedExecutable(ctx, found, commands)
	if err != nil {
		return err
	}
	if err := commands.Run(ctx, found.brew,
		[]string{"upgrade", "--formula", "--no-ask", homebrewFormula}, nil, nil, stdout, stderr); err != nil {
		return fmt.Errorf("brew upgrade: %w", err)
	}
	line, err := commands.Output(ctx, managedPath, "version")
	if err != nil {
		return fmt.Errorf("verify the upgraded Homebrew executable: %w", err)
	}
	if !reportsVersion(line, tag) {
		return fmt.Errorf("Homebrew completed but %s does not report version %s", managedPath, tag)
	}
	return nil
}

// homebrewManagedExecutable は、現在の実行ファイルを所有するformulaの安定パスを返す。
// Cellar内のversion付きパスをunitへ保存するとupgrade後に古いkegへ固定されるため、
// serviceとupdateの両方がこの照合済みパスを使う。
func homebrewManagedExecutable(ctx context.Context, found installation, commands updateCommands) (string, error) {
	prefixOutput, err := commands.Output(ctx, found.brew, "--prefix", "--installed", homebrewFormula)
	if err != nil {
		return "", fmt.Errorf("Homebrew does not report %s as installed: %w", homebrewFormula, err)
	}
	prefix := strings.TrimSpace(string(prefixOutput))
	if prefix == "" || !filepath.IsAbs(prefix) || strings.ContainsAny(prefix, "\r\n") {
		return "", errors.New("Homebrew returned an invalid formula prefix")
	}
	managedPath := filepath.Join(prefix, "bin", "sshc")
	managed, err := os.Stat(managedPath)
	if err != nil {
		return "", fmt.Errorf("inspect Homebrew's sshc: %w", err)
	}
	running, err := os.Stat(found.executable)
	if err != nil {
		return "", fmt.Errorf("inspect this sshc: %w", err)
	}
	if !os.SameFile(managed, running) {
		return "", fmt.Errorf("Homebrew manages %s, not the running executable %s", managedPath, found.executable)
	}
	return managedPath, nil
}

func runTaggedInstaller(ctx context.Context, found installation, tag string, client *http.Client, commands updateCommands, stdout, stderr io.Writer) error {
	if runtime.GOOS == "windows" {
		return errors.New("install.sh updates are not supported on Windows")
	}
	if stable, ok := selfupdate.StableTag(tag); !ok || stable != tag {
		return fmt.Errorf("refuse installer tag %q", tag)
	}
	url := "https://raw.githubusercontent.com/aida0710/sshc/" + tag + "/install.sh"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download the tagged installer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download the tagged installer: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxInstallerSize {
		return errors.New("the tagged installer is unexpectedly large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxInstallerSize+1))
	if err != nil {
		return fmt.Errorf("read the tagged installer: %w", err)
	}
	if len(body) > maxInstallerSize {
		return errors.New("the tagged installer is unexpectedly large")
	}

	temporary, err := os.CreateTemp("", "sshc-install-*.sh")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	shell, err := exec.LookPath("sh")
	if err != nil {
		return errors.New("sh is required to run the install.sh updater")
	}
	environment := replaceEnvironment(os.Environ(), map[string]string{
		"SSHC_VERSION":     tag,
		"SSHC_INSTALL_DIR": filepath.Dir(found.executable),
	})
	if err := commands.Run(ctx, shell, []string{temporaryPath}, environment, nil, stdout, stderr); err != nil {
		return fmt.Errorf("install.sh: %w", err)
	}
	matched, err := shellReceiptMatches(found.executable)
	if err != nil {
		return fmt.Errorf("verify the updated install receipt: %w", err)
	}
	if !matched {
		return errors.New("install.sh completed without a valid install receipt")
	}
	line, err := commands.Output(ctx, found.executable, "version")
	if err != nil {
		return fmt.Errorf("verify the updated executable: %w", err)
	}
	if !reportsVersion(line, tag) {
		return fmt.Errorf("install.sh completed but the executable does not report version %s", tag)
	}
	return nil
}

func reportsVersion(line []byte, tag string) bool {
	fields := bytes.Fields(line)
	if len(fields) != 3 || string(fields[0]) != "sshc" {
		return false
	}
	reported, ok := selfupdate.StableTag(string(fields[1]))
	return ok && reported == tag
}

func replaceEnvironment(current []string, replacements map[string]string) []string {
	next := make([]string, 0, len(current)+len(replacements))
	for _, entry := range current {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := replacements[name]; replaced {
				continue
			}
		}
		next = append(next, entry)
	}
	for name, value := range replacements {
		next = append(next, name+"="+value)
	}
	return next
}
