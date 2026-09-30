//go:build !windows

package sshclient

import (
	"errors"
	"net"
	"os"
	"syscall"
	"testing"

	"sshc/internal/terminal"
)

// 転送の理由は、画面が訳せる決まった語で残る。Go のエラーの文は残さない。
func TestAListenFailureIsRecordedAsAWordTheScreenTranslates(t *testing.T) {
	listenError := func(errno syscall.Errno) error {
		return &net.OpError{Op: "listen", Net: "tcp", Err: os.NewSyscallError("bind", errno)}
	}
	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"port in use":        {listenError(syscall.EADDRINUSE), terminal.ForwardProblemAddressInUse},
		"privileged port":    {listenError(syscall.EACCES), terminal.ForwardProblemPermissionDenied},
		"anything else":      {listenError(syscall.EADDRNOTAVAIL), terminal.ForwardProblemFailed},
		"not a system error": {errors.New("closed"), terminal.ForwardProblemFailed},
	} {
		if got := listenProblem(test.err); got != test.want {
			t.Errorf("%s: listenProblem = %q, want %q", name, got, test.want)
		}
	}
}
