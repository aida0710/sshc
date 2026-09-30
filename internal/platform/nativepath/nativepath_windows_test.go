//go:build windows

package nativepath

import (
	"strings"
	"testing"
)

func TestWindowsAcceptsDriveAndOrdinaryUNCRoots(t *testing.T) {
	for _, path := range []string{
		`C:\Users\A\.ssh\config`,
		`C:/Users/A/.ssh/config`,
		`\\server\share\A\.ssh\config`,
	} {
		if !Supported(path) {
			t.Errorf("Supported(%q) = false, want true", path)
		}
	}
}

// `C:config` はどのディレクトリからの相対か、`\config` はどのボリュームかを
// 表記が言っていない。絶対として扱えば、たまたま別のファイルが読まれる。
func TestWindowsRefusesDriveRelativeAndRootRelativeSpellings(t *testing.T) {
	for _, path := range []string{`C:config`, `\config`, `config`, `..\config`} {
		if Supported(path) {
			t.Errorf("Supported(%q) = true, want false", path)
		}
	}
}

func TestWindowsCaseAliasesAreTheSamePlace(t *testing.T) {
	root := `C:\Users\A\.ssh`
	for _, candidate := range []string{
		`c:\users\a\.ssh`,
		`C:\USERS\A\.SSH\config`,
		`c:/Users/A/.ssh/conf.d/work.conf`,
	} {
		if !Contains(root, candidate) {
			t.Errorf("Contains(%q, %q) = false, want true", root, candidate)
		}
	}
	if Identity(root+`\config`) != Identity(`c:\users\a\.ssh\CONFIG`) {
		t.Fatal("Windows case aliases produced different identities")
	}
}

// 文中のパスを探すときも、Contains と同じく大小文字と区切り文字を同一視する。
// 返す長さは text 側の綴りのバイト数である。
func TestWindowsMatchPrefixFindsCaseAndSeparatorAliases(t *testing.T) {
	root := `C:\Users\A\.ssh`
	for _, text := range []string{`c:\users\a\.ssh\config`, `C:/USERS/A/.SSH/config`} {
		length, ok := MatchPrefix(text, root)
		if !ok || length != len(`c:\users\a\.ssh`) {
			t.Errorf("MatchPrefix(%q, %q) = %d, %v, want %d, true", text, root, length, ok, len(`c:\users\a\.ssh`))
		}
	}
	if _, ok := MatchPrefix(`C:\Users\B\.ssh\config`, root); ok {
		t.Error("a different account matched the root")
	}
}

func TestWindowsSeparatesOtherVolumesAndShares(t *testing.T) {
	for _, test := range []struct{ root, candidate string }{
		{`C:\Users\A\.ssh`, `D:\Users\A\.ssh\config`},
		{`C:\Users\A\.ssh`, `C:\Users\A\.ssh-other\config`},
		{`\\server\share\A\.ssh`, `\\server\other\A\.ssh\config`},
		{`\\server\share\A\.ssh`, `\\elsewhere\share\A\.ssh\config`},
	} {
		if Contains(test.root, test.candidate) {
			t.Errorf("Contains(%q, %q) = true, want false", test.root, test.candidate)
		}
	}
}

func TestWindowsUNCCaseAliasesAreTheSameShare(t *testing.T) {
	root := `\\server\share\A\.ssh`
	if !Contains(root, `\\SERVER\SHARE\a\.ssh\config`) {
		t.Fatal("a UNC case alias was reported outside the root")
	}
}

// Identity と Contains は同じ正規化規則を使用する必要がある。
//
// 割れる向き次第では、別のファイルが同じ鍵に畳まれ、二つ目の Include が
// 一度も読まれないまま消える。
func TestWindowsIdentityAgreesWithContainmentFolding(t *testing.T) {
	root := `C:\Users\A\.ssh`
	for _, pair := range [][2]string{
		{"config", "CONFIG"},
		{"Config", "conFIG"},
		// simple fold の軌道が三つ以上ある組。ToLower はここで包含判断とずれる。
		{"si", "sİ"},
		{"stra\u00dfe", "STRA\u00dfE"},
		{"\u017foo", "Soo"},
		{"\u212aelvin", "kelvin"},
	} {
		first := root + `\` + pair[0]
		second := root + `\` + pair[1]
		sameIdentity := Identity(first) == Identity(second)
		sameFold := strings.EqualFold(first, second)
		if sameIdentity != sameFold {
			t.Errorf("Identity(%q)==Identity(%q) is %v, EqualFold is %v", first, second, sameIdentity, sameFold)
		}
	}
}

// 別のファイルが同じ鍵に畳まれないことを、はっきり表明する。
func TestWindowsIdentitySeparatesDistinctFiles(t *testing.T) {
	root := `C:\Users\A\.ssh`
	if Identity(root+`\alpha`) == Identity(root+`\beta`) {
		t.Fatal("distinct names share an identity")
	}
}

// Windows の filepath.Rel は、要素に `C:` を含むパスに対して `C:\x` のような絶対
// パスを返す。それを root の下と読むと、層によって内と外の答えが分かれる。
func TestWindowsAComponentThatLooksLikeADriveIsOutsideTheRoot(t *testing.T) {
	root := `C:\Users\A\.ssh`
	candidate := root + `\C:\x`
	if relative, inside := Relative(root, candidate); inside {
		t.Errorf("Relative(%q, %q) = (%q, true), want outside", root, candidate, relative)
	}
	if relative, inside := RelativeBelow(root, candidate); inside {
		t.Errorf("RelativeBelow(%q, %q) = (%q, true), want outside", root, candidate, relative)
	}
	if relative, inside := RelativeSlash(root, candidate); inside {
		t.Errorf("RelativeSlash(%q, %q) = (%q, true), want outside", root, candidate, relative)
	}
}
