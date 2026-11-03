package terminal

import "regexp"

// executablePathTitle は、自分で題名を付けていないプログラムに Windows のコンソールが付ける題名である。
// 実行ファイルの絶対パスで、管理者なら前に翻訳された語（「管理者: 」「Administrator: 」）が付き、
// cmd.exe がコマンドを動かしている間は後ろに「 - <コマンド>」が付く。
var executablePathTitle = regexp.MustCompile(`(?i)^(?:[^\\/:：]+\s?[:：]\s*)?[a-z]:\\[^:*?"<>|]*\.exe(?: - (.*))?$`)

// withoutExecutablePath は、題名に実行ファイルのパスしか無ければ、題名が付いていないものとして空を返す。
// cmd.exe がコマンドを動かしているなら、そのコマンドを返す。パスは長くて一覧の幅に収まらず、セッションの
// 見分けにも役立たない。長さの上限（MaxTitle）で切る前に見る。切ったあとは拡張子で見分けられない。
func withoutExecutablePath(title string) string {
	match := executablePathTitle.FindStringSubmatch(title)
	if match == nil {
		return title
	}
	return match[1]
}
