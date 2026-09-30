// Package configresolver は、ワークスペース（~/.ssh）を読む ssh_config の Include
// リゾルバを組み立てる。
//
// storage はディスクの原始操作だけを持ち、ssh_config の解決器（config.Resolver）を
// 知らない。両者を結ぶ組み立てをここに置く。application と diagnostics も同じ
// リゾルバを組むので、合成の根（internal/app）ではなく、どちらからも import できる
// この場所にある。
package configresolver

import (
	"os/user"

	"sshc/internal/config"
	"sshc/internal/platform"
	"sshc/internal/storage"
)

// workspaceLoader は、Include グラフにディスクへの読み取り専用アクセスを与える。
//
// 意図的にワークスペースのルート外のファイルも読む。よそを指す Include を表示する
// ことが設計上求められるからだ。ただしシンボリックリンクをたどることはなく、書き
// 込むこともない。何を変更してよいかを決めるのは Workspace.ResolveForWrite だけで
// ある。
type workspaceLoader struct {
	fileSystem storage.FileSystem
}

func (l workspaceLoader) ReadFile(path string) ([]byte, error) {
	return l.fileSystem.ReadFile(path)
}

func (l workspaceLoader) Glob(pattern string) ([]string, error) {
	return l.fileSystem.Glob(pattern)
}

// ForWorkspace は、ワークスペースのための Include リゾルバを組み立てる。
//
// 供給するパーセントトークンは、接続先ホストが決まる前に確定しているものだけである。
// '%d'、'%u'、'%i' がそれにあたる。'%h' や '%C' は決まっていないので供給せず、それを
// 使う Include は推測されるのではなく非対応として報告される。
func ForWorkspace(workspace *storage.Workspace) config.Resolver {
	tokens := map[byte]string{'d': workspace.Home()}
	// ユーザー名と uid はプロセスの性質であってワークスペースの性質ではないので、
	// Home のように注入するのではなくここで読む。ファイルには触れないため、これで
	// テストが本物のホームディレクトリに届くことはない。読めない環境では、その
	// トークンを供給しないことで、以前と同じく非対応として報告される。
	if current, err := user.Current(); err == nil {
		tokens['u'] = platform.LocalAccountName(current.Username)
		tokens['i'] = current.Uid
	}
	return config.Resolver{
		Loader: workspaceLoader{fileSystem: workspace.FileSystem()},
		Home:   workspace.Home(),
		Root:   workspace.Root(),
		// '~' と '%d' は、与えられたままのホームへ展開され、その結果に対する判断は
		// すべて解決済みのルートに対して行われる。両者を同じファイルに保つのが
		// Normalise である。
		Normalise: workspace.Normalise,
		Tokens:    tokens,
	}
}
