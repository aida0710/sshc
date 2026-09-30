package sftp

import (
	"context"
	"errors"
	"io/fs"
	"path"
)

// boundedTreeWalk は、検索・容量の集計・再帰 chmod が共有する remote の木の走査
// 1 回分を表す。予算（maxSearchDepth 段、maxSearchVisited 項目）、sshc の内部名を
// 飛ばすこと、symlink を辿らないことは walkBoundedTree が決める。予算を超えたときと
// 読めないディレクトリの扱いは、呼び出し側が決める。
type boundedTreeWalk struct {
	root string
	// visit は root の配下の項目を浅い段から順に受け取る。内部名は渡さない。
	// errStopWalk を返すと、走査は失敗せずにそこで終わる。
	visit func(directory string, child fs.FileInfo) error
	// skipUnreadable が nil でなければ、読めないディレクトリを飛ばして続け、その
	// たびに呼ぶ。nil なら、読めなかった時点で走査を失敗にする。
	skipUnreadable func()
}

// errStopWalk は、visit が走査をそこで終えてよいと伝える値。walkBoundedTree の外へは出ない。
var errStopWalk = errors.New("the bounded walk was stopped by its visitor")

// walkBoundedTree は root の配下を段ごとに辿る。深さか項目数の予算を超えると
// ErrTraversalLimit を返す。取消と期限切れは、読めない枝を飛ばす走査でも失敗にする。
func walkBoundedTree(ctx context.Context, remote Remote, walk boundedTreeWalk) error {
	walker := &boundedTreeWalker{remote: remote, walk: walk}
	pending := []string{walk.root}
	for depth := 0; len(pending) > 0; depth++ {
		if depth > maxSearchDepth {
			return ErrTraversalLimit
		}
		var next []string
		for _, directory := range pending {
			subdirectories, err := walker.visitDirectory(ctx, directory)
			if errors.Is(err, errStopWalk) {
				return nil
			}
			if err != nil {
				return err
			}
			next = append(next, subdirectories...)
		}
		pending = next
	}
	return nil
}

// boundedTreeWalker は 1 回の走査で数えた項目数を持つ。
type boundedTreeWalker struct {
	remote  Remote
	walk    boundedTreeWalk
	visited int
}

// visitDirectory は directory の子を visit に渡し、次の段で辿るディレクトリを返す。
func (w *boundedTreeWalker) visitDirectory(ctx context.Context, directory string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	children, err := readChildren(ctx, w.remote, directory)
	if err != nil {
		if w.walk.skipUnreadable == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		w.walk.skipUnreadable()
		return nil, nil
	}
	var subdirectories []string
	for _, child := range children {
		if isInternalName(child.Name()) {
			continue
		}
		w.visited++
		if w.visited > maxSearchVisited {
			return nil, ErrTraversalLimit
		}
		if err := w.walk.visit(directory, child); err != nil {
			return nil, err
		}
		// readChildren は symlink を辿らない情報を返すので、symlink の IsDir は偽になる。
		if child.IsDir() {
			subdirectories = append(subdirectories, path.Join(directory, child.Name()))
		}
	}
	return subdirectories, nil
}
