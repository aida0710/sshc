//go:build windows

package nativepath

import "testing"

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
// 綴りが言っていない。絶対として扱えば、たまたま別のファイルが読まれる。
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
