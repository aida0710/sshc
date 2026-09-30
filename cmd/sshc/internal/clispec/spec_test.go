package clispec

import (
	"fmt"
	"strings"
	"testing"

	sftpcore "sshc/internal/sftp"
)

// engine の既定値を変えたら、sftp のヘルプもその値を書く。
func TestSFTPHelpStatesTheEngineDefaults(t *testing.T) {
	var help string
	for _, command := range Commands {
		if command.Name == "sftp" {
			help = command.Help
		}
	}
	for _, want := range []string{
		fmt.Sprintf("(engine default %d)", sftpcore.DefaultLargeFileThreshold>>20),
		fmt.Sprintf("(engine default %d; 1 disables)", sftpcore.DefaultLargeFileParallelism),
		fmt.Sprintf("(engine default %d)", sftpcore.DefaultLargeFileChunkBytes>>20),
	} {
		if !strings.Contains(help, want) {
			t.Errorf("sftp help does not state %q:\n%s", want, help)
		}
	}
}
