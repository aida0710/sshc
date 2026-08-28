//go:build !windows

package buildcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScriptPublishesAReceiptForTheExactBinary(t *testing.T) {
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	commands := filepath.Join(root, "commands")
	installDirectory := filepath.Join(root, "installed")
	for _, directory := range []string{fixtures, commands, installDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh only supports Linux and macOS")
	}
	assetName := "sshc-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOARCH == "386" {
		t.Skip("the release does not publish linux/386")
	}
	asset := filepath.Join(fixtures, assetName)
	assetBody := "#!/bin/sh\nprintf '%s\\n' 'sshc v9.8.7 " + runtime.GOOS + "/" + runtime.GOARCH + "'\n"
	if err := os.WriteFile(asset, []byte(assetBody), 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(assetBody))
	if err := os.WriteFile(filepath.Join(fixtures, "checksums.txt"),
		[]byte(hex.EncodeToString(digest[:])+"  "+assetName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// install.shのHTTP境界だけを偽物にし、checksum・same-directory staging・receipt
	// publicationは実際のshellで通す。
	fakeCurl := `#!/bin/sh
set -eu
url=""
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */checksums.txt) cp "$SSHC_TEST_FIXTURES/checksums.txt" "$output" ;;
  */sshc-*) cp "$SSHC_TEST_FIXTURES/${url##*/}" "$output" ;;
  *) exit 22 ;;
esac
`
	if err := os.WriteFile(filepath.Join(commands, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", script)
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "SHELL=") {
			continue
		}
		environment = append(environment, entry)
	}
	command.Env = append(environment,
		"PATH="+commands+":/usr/bin:/bin",
		"HOME="+filepath.Join(root, "home"),
		"SSHC_VERSION=v9.8.7",
		"SSHC_INSTALL_DIR="+installDirectory,
		"SSHC_TEST_FIXTURES="+fixtures,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh = %v\n%s", err, output)
	}
	target := filepath.Join(installDirectory, "sshc")
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != assetBody {
		t.Fatal("installed binary is not the verified fixture")
	}

	receiptBody, err := os.ReadFile(filepath.Join(installDirectory, ".sshc-install-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		SchemaVersion int    `json:"schemaVersion"`
		Manager       string `json:"manager"`
		Repository    string `json:"repository"`
		Version       string `json:"version"`
		SHA256        string `json:"sha256"`
	}
	if err := json.Unmarshal(receiptBody, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.Manager != "install.sh" || receipt.Repository != "aida0710/sshc" ||
		receipt.Version != "v9.8.7" || receipt.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("receipt = %#v", receipt)
	}
	entries, err := os.ReadDir(installDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".sshc.install.") || strings.HasPrefix(entry.Name(), ".sshc.receipt.") {
			t.Errorf("staging file remains: %s", entry.Name())
		}
	}
}
