package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sshc/internal/releasecheck"
	"sshc/internal/strictjson"
)

// sshcを入れる方法（Homebrew、install.sh、install.ps1）が、どこから何を入れるか。
const (
	homebrewTap       = "aida0710/tap"
	homebrewFormula   = homebrewTap + "/sshc"
	installRepository = "aida0710/sshc"
	// receiptFileNameは、インストーラーが実行ファイルの隣に置く、入れた版の記録である。
	receiptFileName = ".sshc-install-receipt.json"
)

type installManager uint8

const (
	managerUnknown installManager = iota
	managerHomebrew
	managerShell
	managerPowerShell
)

func (manager installManager) installerName() string {
	if manager == managerPowerShell {
		return "install.ps1"
	}
	return "install.sh"
}

type installation struct {
	manager    installManager
	executable string
	brew       string
}

type installReceipt struct {
	SchemaVersion int    `json:"schemaVersion"`
	Manager       string `json:"manager"`
	Repository    string `json:"repository"`
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
}

// detectInstallation は、実行ファイルを入れた管理元を判定する。Homebrew の Cellar の
// 中にあるか、インストーラーの記録と中身が一致するときだけ管理元が分かったとみなし、
// それ以外（source からの build、手で置いたもの）は managerUnknown にする。
func detectInstallation(executable string) (installation, error) {
	return detectInstallationForOS(executable, runtime.GOOS)
}

func detectInstallationForOS(executable, goos string) (installation, error) {
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return installation{}, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return installation{}, err
	}
	manager, installer, name := managerShell, "install.sh", "sshc"
	if goos == "windows" {
		manager, installer, name = managerPowerShell, "install.ps1", "sshc.exe"
	} else if brew, ok := homebrewForExecutable(resolved); ok {
		return installation{manager: managerHomebrew, executable: resolved, brew: brew}, nil
	}
	if filepath.Base(resolved) == name || goos == "windows" && strings.EqualFold(filepath.Base(resolved), name) {
		matched, err := installerReceiptMatches(resolved, installer)
		if err != nil {
			return installation{}, err
		}
		if matched {
			return installation{manager: manager, executable: resolved}, nil
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

func installerReceiptMatches(executable, installer string) (bool, error) {
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
	contents, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var receipt installReceipt
	if err := strictjson.Decode(contents, &receipt); errors.Is(err, strictjson.ErrTrailingData) {
		return false, fmt.Errorf("%s contains trailing data", path)
	} else if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if receipt.SchemaVersion != 1 || receipt.Manager != installer || receipt.Repository != installRepository {
		return false, fmt.Errorf("%s does not describe an sshc %s installation", path, installer)
	}
	if _, ok := releasecheck.StableTag(receipt.Version); !ok || len(receipt.SHA256) != sha256.Size*2 {
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
		return false, fmt.Errorf("%s does not match the installed executable; install sshc again with %s, or remove the receipt if you replaced the executable yourself", path, installer)
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

// managedInstallationExecutable は、管理元が版を入れ替えても同じ場所を指す実行ファイルの
// パスを返す。管理元が分からない実行ファイルは断る。
func managedInstallationExecutable(ctx context.Context, found installation, commands installationCommands) (string, error) {
	switch found.manager {
	case managerHomebrew:
		return homebrewManagedExecutable(ctx, found, commands)
	case managerShell, managerPowerShell:
		return found.executable, nil
	default:
		if runtime.GOOS == "windows" {
			return "", fmt.Errorf("%s is not managed by sshc's PowerShell installer; install it first with: %s", found.executable, windowsInstallerCommand)
		}
		return "", fmt.Errorf("%s is not managed by Homebrew or sshc's install.sh", found.executable)
	}
}

// homebrewManagedExecutable は、現在の実行ファイルを所有するformulaの安定パスを返す。
// Cellar内のversion付きパスをunitへ保存するとupgrade後に古いkegへ固定されるため、
// serviceとupdateの両方がこの照合済みパスを使う。
func homebrewManagedExecutable(ctx context.Context, found installation, commands installationCommands) (string, error) {
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
