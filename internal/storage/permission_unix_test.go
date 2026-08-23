//go:build unix

// 保たれるべき mode があるのは Unix である。
//
// Windows でこの fixture を作ると、別のものが出来る。あちらの Chmod は
// `0400` を FILE_ATTRIBUTE_READONLY に写し、その属性の付いたファイルは
// MOVEFILE_REPLACE_EXISTING で置き換えられない。つまり向こうで試せるのは
// 「厳しい mode が保たれるか」ではなく「読み取り専用属性の付いた設定を
// 保存できるか」であり、結果は今のところできない。利用者が ~/.ssh/config に
// 読み取り専用を立てていると save が Access is denied で断られる。その属性を
// 外して書き戻す判断は、journal を持つトランザクションの永続性の話であって
// 移植の話ではない。
package storage

import (
	"os"
	"testing"
)

func TestCommitPreservesStricterPermissions(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "strict.conf", "Host old\n", 0o400)
	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes:   []Change{{Path: path, Contents: []byte("Host new\n"), Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host old\n"))}}},
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Fatalf("permission = %v, want 0400", info.Mode().Perm())
	}
}
