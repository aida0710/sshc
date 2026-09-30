package sshclient

import (
	"strings"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

func describeAlgorithmOffers(trace *tracer, config *ssh.ClientConfig) {
	if !trace.enabled(connectionlog.Full) {
		return
	}
	// Read the library's actual defaults without changing the connection policy.
	defaults := config.Config
	defaults.SetDefaults()
	trace.say(connectionlog.Full, "提示する鍵交換アルゴリズム：%s", strings.Join(defaults.KeyExchanges, ", "))
	trace.say(connectionlog.Full, "提示する暗号：%s", strings.Join(defaults.Ciphers, ", "))
	trace.say(connectionlog.Full, "提示するMAC：%s", strings.Join(defaults.MACs, ", "))
}

func describeNegotiatedAlgorithms(trace *tracer, algorithms ssh.NegotiatedAlgorithms) {
	trace.say(connectionlog.Detailed, "採用された鍵交換：%s、ホスト鍵：%s", algorithms.KeyExchange, algorithms.HostKey)
	trace.say(connectionlog.Detailed, "クライアント → サーバー：暗号%s、MAC %s", algorithms.Write.Cipher, negotiatedMAC(algorithms.Write))
	trace.say(connectionlog.Detailed, "サーバー → クライアント：暗号%s、MAC %s", algorithms.Read.Cipher, negotiatedMAC(algorithms.Read))
}

func negotiatedMAC(algorithms ssh.DirectionAlgorithms) string {
	if algorithms.MAC == "" {
		return "暗号に内蔵（AEAD）"
	}
	return algorithms.MAC
}

func traceAuthentication(trace *tracer) ssh.ClientAuthCallback {
	if !trace.enabled(connectionlog.Detailed) {
		return nil
	}
	started := false
	return func(auth *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
		if !started {
			trace.say(connectionlog.Full, "認証前のサーバーのSSHバージョン：%s", terminal.DisplayText(string(auth.Metadata.ServerVersion()), maxBannerLineRunes))
			describeNegotiatedAlgorithms(trace, auth.Algorithms)
			started = true
		}
		trace.say(connectionlog.Detailed, "サーバーが受け付ける認証方式：%s", strings.Join(auth.AllowedMethods, ", "))
		if len(auth.TriedMethods) > 0 {
			trace.say(connectionlog.Detailed, "成功しなかった認証方式：%s", strings.Join(auth.TriedMethods, ", "))
		}
		if len(auth.PartialSuccessMethods) > 0 {
			trace.say(connectionlog.Detailed, "追加認証待ち（成功済み：%s）", strings.Join(auth.PartialSuccessMethods, ", "))
		}
		// nil preserves the library's selection from ClientConfig.Auth.
		return nil, nil
	}
}
