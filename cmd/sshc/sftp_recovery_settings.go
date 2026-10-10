package main

import sftpcore "sshc/internal/sftp"

func readSFTPRecoverySettings(arguments commandArguments, called *sftpInvocation) error {
	for _, option := range []struct {
		name    string
		maximum int
		value   **int
	}{
		{name: "--speed-limit", maximum: int(sftpcore.MaxTransferSpeedBytesPerSecond >> 10), value: &called.SpeedLimitKiB},
		{name: "--reconnect-attempts", maximum: sftpcore.MaxReconnectAttempts, value: &called.ReconnectAttempts},
	} {
		if !arguments.has(option.name) {
			continue
		}
		value, err := arguments.integer(option.name, integerBounds{minimum: 0, maximum: option.maximum, kind: "a number"}, 0)
		if err != nil {
			return err
		}
		*option.value = &value
	}
	return nil
}
