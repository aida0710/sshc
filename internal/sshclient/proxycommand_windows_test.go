package sshclient

import (
	"os/exec"
	"testing"
)

func TestProxyCommandUsesCmdExeRawCommandLine(t *testing.T) {
	const line = `"C:\\Program Files\\helper.exe" --stdio`
	command := exec.Command(`C:\\Windows\\System32\\cmd.exe`, "/d", "/s", "/c", line)
	configureProxyCommandProcess(command, line)
	if len(command.Args) != 0 {
		t.Fatalf("Args = %#v, want raw CmdLine only", command.Args)
	}
	want := `/d /s /c "` + line + `"`
	if command.SysProcAttr == nil || command.SysProcAttr.CmdLine != want {
		t.Fatalf("CmdLine = %#v, want %q", command.SysProcAttr, want)
	}
}
