package validate_test

import (
	"strings"
	"testing"

	"sshc/internal/validate"
)

// 先頭の `-` を断る。
//
// グループ名はディレクトリ名になり、その表記は Include のパスに現れる。alias で
// 同じ理由から拒んでいるのと同じ危うさである。長らく Go はこれを受け入れ、画面だけが
// 断っていた。画面の方が厳しいという、利用者に直しようのない食い違いだった。
func TestAGroupNameCannotStartWithADash(t *testing.T) {
	if validate.GroupName("-foo") == nil {
		t.Error("a group name starting with a dash was accepted")
	}
	if err := validate.GroupName("foo-bar"); err != nil {
		t.Errorf("a dash inside the name was refused: %v", err)
	}
}

// 予約語は、OpenSSH とこのアプリケーションが ~/.ssh の中で既に意味を与えている
// 名前である。画面にはこのうち 6 つしか無かった。
func TestEveryReservedNameIsRefused(t *testing.T) {
	for _, name := range validate.ReservedNames {
		if validate.GroupName(name) == nil {
			t.Errorf("the reserved name %q was accepted", name)
		}
		// 既定の macOS ボリュームは "Config" と "config" を同じ項目として扱う。
		if validate.GroupName(strings.ToUpper(name)) == nil {
			t.Errorf("the reserved name %q was accepted in upper case", name)
		}
	}
}

// パターンは両方の側が同じ表記を読む。Go の RE2 と JavaScript が同じ意味で読む
// 書き方だけを使う。後方参照も先読みも、片方にしか無い。
func TestThePatternsStayInTheSharedSubset(t *testing.T) {
	for name, pattern := range map[string]string{
		"group segment": validate.GroupSegmentPattern,
		"alias":         validate.AliasPattern,
		"hostname":      validate.HostnamePattern,
	} {
		for _, forbidden := range []string{`(?`, `\b`, `\k`, `(?<`} {
			if strings.Contains(pattern, forbidden) {
				t.Errorf("%s pattern uses %q, which Go and JavaScript do not read alike", name, forbidden)
			}
		}
	}
}

// 鍵のファイル名にも同じ一覧が効く。
//
// どちらも ~/.ssh の直下にその表記を作る操作である。`keys` という名前の鍵は、あとで
// グループを作るときに要る `keys/` と同じ場所を取り合う。一覧が 2 つあった間、
// 鍵の側にはその 2 つが無かった。
func TestTheReservedListCoversTheDirectoriesThisApplicationOwns(t *testing.T) {
	for _, name := range []string{"keys", "connections"} {
		if !validate.Reserved(name) {
			t.Errorf("%q is a directory this application creates under ~/.ssh, but it is not reserved", name)
		}
	}
}
