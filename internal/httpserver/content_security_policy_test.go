package httpserver

import (
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"sshc/internal/ui"
)

// pageDirective は、ページに付く CSP の指令 name の値を空白で区切って返す。
// 指令が無ければ ok は false になる。
func pageDirective(t *testing.T, name string) (values []string, ok bool) {
	t.Helper()
	header := http.Header{}
	setSecurityHeaders(header, false)
	for _, directive := range strings.Split(header.Get("Content-Security-Policy"), ";") {
		fields := strings.Fields(directive)
		if len(fields) > 0 && fields[0] == name {
			return fields[1:], true
		}
	}
	return nil, false
}

// SFTP のエディタ（Monaco Editor）は、行の HTML を editorViewLayer の policy を通して
// 入れる。この名前を許さないと本文が 1 行も描かれない（v0.24.0 から起きていた）。
// 名前を許しても、文字列のままの代入を止める require-trusted-types-for 'script' と、
// 作った名前を後から作り直させない（'allow-duplicates' の無い）trusted-types は保つ。
func TestThePagePolicyLetsTheEditorDrawItsLinesAndStillRequiresTrustedTypes(t *testing.T) {
	required, ok := pageDirective(t, "require-trusted-types-for")
	if !ok || !slices.Equal(required, []string{"'script'"}) {
		t.Errorf("require-trusted-types-for = %q (present %v), want 'script'", required, ok)
	}
	allowed, ok := pageDirective(t, "trusted-types")
	if !ok {
		t.Fatal("the page policy has no trusted-types directive")
	}
	if !slices.Contains(allowed, "editorViewLayer") {
		t.Errorf("trusted-types = %q, want it to allow Monaco's editorViewLayer", allowed)
	}
	for _, keyword := range []string{"'allow-duplicates'", "*", "'none'"} {
		if slices.Contains(allowed, keyword) {
			t.Errorf("trusted-types carries %s: %q", keyword, allowed)
		}
	}
}

// trustedTypesPolicyCreation は、policy を作る呼び出しの第 1 引数に書かれた名前を拾う。
// Monaco の createTrustedTypesPolicy('editorViewLayer', { createHTML: … }) も、
// main.tsx の createPolicy("sshc-service-worker", { createScriptURL: … }) も、
// 最小化のあとはこの形で残る（引用符は ` ' " のどれか）。
var trustedTypesPolicyCreation = regexp.MustCompile("[`'\"]([A-Za-z][A-Za-z0-9_-]*)[`'\"]\\s*,\\s*\\{\\s*create(?:HTML|Script|ScriptURL)\\b")

// domPurifyPolicyName は、Monaco が同梱する DOMPurify の policy の名前を拾う。DOMPurify は
// 名前を 'dompurify' と接尾辞をつないで組み立てるので、作る呼び出しの引数には名前が
// 現れない。接尾辞は読み込んだ script 要素の data-tt-policy-suffix から取り、ES module
// として読み込む sshc の画面では付かない。
var domPurifyPolicyName = regexp.MustCompile("[`'\"](dompurify)[`'\"]\\s*\\+")

// trustedTypesPoliciesTheEmbeddedUICreates は、埋め込んだ画面のスクリプトが作る
// Trusted Types の policy の名前を、重複を除いて並べて返す。
func trustedTypesPoliciesTheEmbeddedUICreates(t *testing.T) []string {
	t.Helper()
	assets, err := ui.FS()
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := fs.Glob(assets, "assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, script := range scripts {
		source, err := fs.ReadFile(assets, script)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range []*regexp.Regexp{trustedTypesPolicyCreation, domPurifyPolicyName} {
			for _, match := range pattern.FindAllSubmatch(source, -1) {
				names = append(names, string(match[1]))
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// trusted-types に並べる名前は、画面が実際に作る policy と一致させる。Monaco を上げて
// policy の名前が増えれば、許していない名前の作成が止められてエディタが壊れる。
// 使われなくなった名前を残せば、ほかのスクリプトがその名前で policy を作れる。
// どちらもこのテストで気付けるように、web をビルドし直した internal/ui/dist と照合する。
func TestThePagePolicyAllowsExactlyTheTrustedTypesPoliciesTheEmbeddedUICreates(t *testing.T) {
	created := trustedTypesPoliciesTheEmbeddedUICreates(t)
	allowed, ok := pageDirective(t, "trusted-types")
	if !ok {
		t.Fatal("the page policy has no trusted-types directive")
	}
	allowed = slices.Clone(allowed)
	slices.Sort(allowed)
	if !slices.Equal(created, allowed) {
		t.Errorf("the embedded UI creates %q,\nthe page policy allows %q", created, allowed)
	}
}
