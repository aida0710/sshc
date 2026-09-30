package main

import (
	"strconv"
	"testing"
	"time"

	sftpcore "sshc/internal/sftp"
)

// CLI の分割転送の範囲は engine の定数から作る。engine の範囲を変えたら、CLI もその
// まま同じ範囲を受け、外側だけを断る。
func TestSFTPOptionsAcceptExactlyTheEngineRange(t *testing.T) {
	for _, test := range []struct {
		flag             string
		minimum, maximum int
		settings         bool
	}{
		{flag: "--jobs", minimum: 1, maximum: sftpcore.MaxTransferConcurrency},
		{flag: "--split-size", minimum: int(sftpcore.MinLargeFileThreshold >> 20), maximum: int(sftpcore.MaxLargeFileThreshold >> 20), settings: true},
		{flag: "--split-jobs", minimum: 1, maximum: sftpcore.MaxLargeFileParallelism, settings: true},
		{flag: "--chunk-size", minimum: int(sftpcore.MinLargeFileChunkBytes >> 20), maximum: int(sftpcore.MaxLargeFileChunkBytes >> 20), settings: true},
	} {
		prefixes := [][]string{{"sshc", "sftp", "get", "server-a", "/a", "b"}}
		if test.settings {
			prefixes = append(prefixes, []string{"sshc", "sftp", "settings"})
		}
		for _, prefix := range prefixes {
			for _, value := range []int{test.minimum, test.maximum} {
				argv := append(append([]string(nil), prefix...), test.flag, strconv.Itoa(value))
				if _, err := parseInvocation(argv); err != nil {
					t.Errorf("parseInvocation(%q) = %v, want the engine limit accepted", argv, err)
				}
			}
			for _, value := range []int{test.minimum - 1, test.maximum + 1} {
				argv := append(append([]string(nil), prefix...), test.flag, strconv.Itoa(value))
				if _, err := parseInvocation(argv); err == nil {
					t.Errorf("parseInvocation(%q) accepted a value outside the engine range", argv)
				}
			}
		}
	}
}

func TestSFTPSettingsResponseIsValidAtEveryEngineLimit(t *testing.T) {
	lowest := sftpCLITransferQueue{
		MaxConcurrent: 1, ClearCompletedAfterSeconds: 0,
		LargeFileThresholdBytes: sftpcore.MinLargeFileThreshold, LargeFileParallelism: 1,
		LargeFileChunkBytes: sftpcore.MinLargeFileChunkBytes,
	}
	highest := sftpCLITransferQueue{
		MaxConcurrent:              sftpcore.MaxTransferConcurrency,
		ClearCompletedAfterSeconds: int(sftpcore.MaxClearCompletedAfter / time.Second),
		LargeFileThresholdBytes:    sftpcore.MaxLargeFileThreshold, LargeFileParallelism: sftpcore.MaxLargeFileParallelism,
		LargeFileChunkBytes: sftpcore.MaxLargeFileChunkBytes,
	}
	for name, settings := range map[string]sftpCLITransferQueue{"lowest": lowest, "highest": highest} {
		if !validSFTPCLITransferSettings(settings) {
			t.Errorf("%s engine settings %#v were refused", name, settings)
		}
	}

	outside := []func(*sftpCLITransferQueue){
		func(settings *sftpCLITransferQueue) { settings.MaxConcurrent++ },
		func(settings *sftpCLITransferQueue) { settings.ClearCompletedAfterSeconds++ },
		func(settings *sftpCLITransferQueue) { settings.LargeFileThresholdBytes++ },
		func(settings *sftpCLITransferQueue) { settings.LargeFileParallelism++ },
		func(settings *sftpCLITransferQueue) { settings.LargeFileChunkBytes++ },
	}
	for index, change := range outside {
		settings := highest
		change(&settings)
		if validSFTPCLITransferSettings(settings) {
			t.Errorf("case %d: settings above the engine limit %#v were accepted", index, settings)
		}
	}
}
