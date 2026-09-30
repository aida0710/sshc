package sshmatch_test

import (
	"testing"

	"sshc/internal/sshmatch"
)

func TestPatternFollowsOpenSSHMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"bastion", "bastion", true},
		// match_pattern は大文字小文字を区別する。Host BASTION だけを持つ設定に
		// `ssh -G bastion` を投げると、そのブロックではなく Host * の値が返る。
		{"BASTION", "bastion", false},
		{"bastion", "BASTION", false},
		{"BASTION", "BASTION", true},
		{"*", "anything", true},
		{"*.internal", "db.internal", true},
		{"*.internal", "internal", false},
		{"web-?", "web-1", true},
		{"web-?", "web-12", false},
		{"a*c*e", "abcde", true},
		{"a*c*e", "abcd", false},
		{"host*", "host", true},
		{"[abc]", "a", false},
	}
	for _, test := range tests {
		if got := sshmatch.Pattern(test.pattern, test.value); got != test.want {
			t.Errorf("Pattern(%q, %q) = %v, want %v", test.pattern, test.value, got, test.want)
		}
	}
}

func TestPatternListRejectsTheValueOnANegatedMatch(t *testing.T) {
	patterns := []string{"*.corp", "!special.corp"}
	if !sshmatch.PatternList(patterns, "build.corp", sshmatch.CaseSensitive) {
		t.Error("build.corp should match *.corp")
	}
	if sshmatch.PatternList(patterns, "special.corp", sshmatch.CaseSensitive) {
		t.Error("a negated pattern must reject the value even when another pattern matches")
	}
	if sshmatch.PatternList([]string{"!special.corp"}, "other.corp", sshmatch.CaseSensitive) {
		t.Error("only negated patterns never match")
	}
}

// match_hostname は値とパターンの ASCII を小文字にしてから比べる。全角の英字は
// OpenSSH の lowercase() でも変わらない。
func TestPatternListIgnoresOnlyASCIICaseWhenAsked(t *testing.T) {
	if !sshmatch.PatternList([]string{"*.EXAMPLE.com"}, "Web.example.COM", sshmatch.IgnoreCase) {
		t.Error("IgnoreCase should fold ASCII letters on both sides")
	}
	if sshmatch.PatternList([]string{"*.EXAMPLE.com"}, "web.example.com", sshmatch.CaseSensitive) {
		t.Error("CaseSensitive should not fold letters")
	}
	if sshmatch.PatternList([]string{"ｗeb"}, "Ｗeb", sshmatch.IgnoreCase) {
		t.Error("IgnoreCase should leave non-ASCII letters alone")
	}
}

func TestLowerASCIILeavesEverythingButASCIILettersAlone(t *testing.T) {
	if got := sshmatch.LowerASCII("Ｗeb.Example-01"); got != "Ｗeb.example-01" {
		t.Errorf("LowerASCII = %q", got)
	}
}
