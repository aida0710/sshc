//go:build windows

package windowsacl_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowsacl"
	"sshc/internal/platform/windowsacl/acltest"
)

// grant は icacls で ACE を一つ足す。本物の ACL を作る。手で組み立てた
// 記述子を自分で読み返しても、確かめられるのは自分の書いた偽物である。
func grant(t *testing.T, path, trustee, rights string) {
	t.Helper()
	command := exec.Command("icacls.exe", path, "/grant", trustee+":"+rights)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("icacls grant %s on %s: %v\n%s", trustee, path, err, output)
	}
}

func writtenKey(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	acltest.WritePrivateFile(t, path, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"))
	return path
}

// 閉じた鍵に印を付けない。常に真の警告は何も伝えず、利用者に警告そのものを
// 無視することを教える。
func TestAKeyClosedToThisUserIsNotReportedAsExposed(t *testing.T) {
	path := writtenKey(t, "id_closed")

	exposed, err := windowsacl.ReadableByOthers(path)

	if err != nil {
		t.Fatalf("ReadableByOthers: %v", err)
	}
	if exposed {
		t.Error("a key written through the private path was reported as exposed")
	}
}

// Windows で検査すべきなのは、別のユーザーに読み取りが
// 許可されていれば、それは露出である。mode ビットは何も言わない。
func TestAKeyReadableByAnotherTrusteeIsReportedAsExposed(t *testing.T) {
	for name, trustee := range map[string]string{
		// SID で指す。表示名は環境で翻訳される。
		"Everyone":            "*S-1-1-0",
		"Authenticated Users": "*S-1-5-11",
		"Users":               "*S-1-5-32-545",
	} {
		t.Run(name, func(t *testing.T) {
			path := writtenKey(t, "id_open")
			grant(t, path, trustee, "(R)")

			exposed, err := windowsacl.ReadableByOthers(path)

			if err != nil {
				t.Fatalf("ReadableByOthers: %v", err)
			}
			if !exposed {
				t.Errorf("a key readable by %s was not reported as exposed", name)
			}
		})
	}
}

// SYSTEM と Administrators は別のユーザーではない。あの二つに読みがあることは
// Windows では普通であり、それを露出と呼べば、すべての鍵に印が付く。
func TestTheUsualAdministrativeTrusteesAreNotOthers(t *testing.T) {
	path := writtenKey(t, "id_admin")
	grant(t, path, "*S-1-5-18", "(F)")     // SYSTEM
	grant(t, path, "*S-1-5-32-544", "(F)") // Administrators

	exposed, err := windowsacl.ReadableByOthers(path)

	if err != nil {
		t.Fatalf("ReadableByOthers: %v", err)
	}
	if exposed {
		t.Error("SYSTEM and Administrators were treated as other people")
	}
}

// 読み以外の権利は露出ではない。書ける相手と読める相手は違う。
func TestATrusteeWhoCannotReadIsNotExposure(t *testing.T) {
	path := writtenKey(t, "id_writeonly")
	// 追記だけを許す。中身は読めない。
	grant(t, path, "*S-1-5-32-545", "(WD)")

	exposed, err := windowsacl.ReadableByOthers(path)

	if err != nil {
		t.Fatalf("ReadableByOthers: %v", err)
	}
	if exposed {
		t.Error("a trustee granted only write was reported as able to read")
	}
}

// 鍵を持っているユーザー本人を「別のユーザー」に分類しない。
//
// 昇格したトークンが作ったファイルの所有者は、その利用者ではなく
// Administrators になる。記述子の所有者だけを安全とみなすと、普通に閉じて
// いる鍵が全部危険と報告される。常に真の警告は、警告を無視することを
// 教える。実機で一度そうなった。
func TestTheCurrentUserIsNeverAnotherPerson(t *testing.T) {
	// 昇格の有無に依らず、ここで作るファイルの DACL は利用者に読みを与える。
	// 所有者がその利用者でなくても、危険にはならない。
	path := filepath.Join(t.TempDir(), "id_ordinary")
	if err := os.WriteFile(path, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	exposed, err := windowsacl.ReadableByOthers(path)

	if err != nil {
		t.Fatalf("ReadableByOthers: %v", err)
	}
	if exposed {
		t.Error("a key an ordinary write produced was reported as exposed; " +
			"the owner of the descriptor is not the only trustee that is not a stranger")
	}
}

// 確かめられなかったことを、安全と言わない。開けない道について
// 「閉じている」と返すのが、この判断で最もしてはならないことである。
func TestAPathThatCannotBeOpenedIsAnError(t *testing.T) {
	_, err := windowsacl.ReadableByOthers(filepath.Join(t.TempDir(), "absent"))

	if err == nil {
		t.Fatal("a missing file was judged rather than refused")
	}
	if _, statErr := os.Stat(filepath.Join(t.TempDir(), "absent")); statErr == nil {
		t.Fatal("the fixture accidentally created the file")
	}
}
