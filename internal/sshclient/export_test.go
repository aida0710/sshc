package sshclient

import "golang.org/x/crypto/ssh"

// Callback は、テストが 1 つの接続のホスト鍵検証を直接呼ぶための入口である。本番の
// 接続は、ホップごとの hostKeyLookup から接続ログ付きの callback を組む。
func (h HostKeys) Callback(target Target, prompt Prompter) ssh.HostKeyCallback {
	return h.lookup(target).callback(prompt, nil)
}

// Algorithms は、テストが 1 つの接続で名乗るホスト鍵アルゴリズムの順を確かめるための
// 入口である。
func (h HostKeys) Algorithms(target Target) []string {
	return h.lookup(target).algorithms()
}
