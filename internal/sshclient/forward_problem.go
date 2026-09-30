package sshclient

import "sshc/internal/terminal"

// listenProblem は、ローカルで待ち受けを開けなかった理由を terminal.ForwardProblem*
// の語にする。OS ごとにエラーの値が違うので、判定は addressInUse と
// listenNotPermitted に分けてある。
func listenProblem(err error) string {
	switch {
	case addressInUse(err):
		return terminal.ForwardProblemAddressInUse
	case listenNotPermitted(err):
		return terminal.ForwardProblemPermissionDenied
	default:
		return terminal.ForwardProblemFailed
	}
}
