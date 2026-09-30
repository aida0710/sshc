package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform/nativepath"
)

func FuzzParseRendersOriginalBytes(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("Host example\n  HostName 10.0.0.1\n"))
	f.Add([]byte("Host=a\r\n\t# comment\nInclude \"conf.d/*.conf\"\n"))
	f.Add([]byte("ProxyCommand \"unterminated\nPort 22"))
	f.Add([]byte(" \t\n\x00\xff=\"\"\n"))
	// argv_split の引用とエスケープ。一重引用符、引用の外の "\ "、語中の引用符、
	// エスケープした引用符を含む Match 行、引用符を含むコメント。
	f.Add([]byte("User 'bob'\nIdentityFile ~/.ssh/my\\ key\nUser b\"o\"b # don't\n"))
	f.Add([]byte("Match exec \"test \\\"a\\\" = \\\"b\\\"\"\n\tUser office\n"))
	f.Fuzz(func(t *testing.T, source []byte) {
		file := Parse(source)
		rendered := file.Render()
		if !bytes.Equal(rendered, source) {
			t.Fatalf("round trip changed bytes: got %q, want %q", rendered, source)
		}
		for index, line := range file.Lines {
			if line.Kind == LineDirective && line.Keyword == "" {
				t.Fatalf("line %d is a directive without a keyword", index)
			}
		}
	})
}

// FuzzRenderArgumentReadsBack は、書き出した引数が argv_split の規則で同じ値に
// 読み戻ることを確かめる。書けない値（改行と NUL）は断られなければならない。
func FuzzRenderArgumentReadsBack(f *testing.F) {
	for _, seed := range []string{"plain", "", "has space", `quote"inside`, "it's", `C:\dir\`, `a\\b`, "#x", "=x"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		argument, err := RenderArgument(" ", value)
		if err != nil {
			if !strings.ContainsAny(value, "\n\r\x00") {
				t.Fatalf("RenderArgument(%q) refused a writable value: %v", value, err)
			}
			return
		}
		arguments, _, ok := splitArguments(argument.Raw + " next")
		if !ok || len(arguments) != 2 || arguments[0].Value != value {
			t.Fatalf("RenderArgument(%q) wrote %q, which reads back as %#v", value, argument.Raw, arguments)
		}
	})
}

// FuzzExpandIncludePattern は、Include の引数をファイルシステムのグロブに変える
// 段階をファズする。設定ファイル中のテキストがパスになるのはエンジン内でここだけ
// なので、相対パスや、正規化されていないパスや、エンジンが推測せざるを得ない
// ホームディレクトリへ展開されるパターンがあれば、リゾルバが読む範囲が広がって
// しまう。
func FuzzExpandIncludePattern(f *testing.F) {
	for _, seed := range []string{
		"conf.d/*.conf",
		"~/.ssh/extra hosts.conf",
		"%d/.ssh/config.d/*",
		"/etc/ssh/ssh_config",
		"~root/config",
		"~",
		"~/",
		"a%",
		"%%literal",
		"%h.conf",
		"../escape.conf",
		"./././x",
		"",
		"a\x00b",
		"a\nb",
	} {
		f.Add(seed)
	}
	// コミット済みのゴールデンフィクスチャからも種を与え、コーパスが実際の設定に
	// 含まれる引数から始まるようにする。
	golden, err := os.ReadFile(filepath.Join("testdata", "golden", "realistic.conf"))
	if err != nil {
		f.Fatal(err)
	}
	for _, line := range Parse(golden).Lines {
		if line.Kind != LineDirective || !EqualKeyword(line.Keyword, "Include") {
			continue
		}
		for _, argument := range line.Arguments {
			f.Add(strings.Trim(argument.Raw, "\""))
		}
	}

	// このターゲットの表明を過去に壊した種は、恒久的に保持する。
	// "~%d" はまずトークンを展開し、そのうえで "~/..." として読まれる。これは、
	// どこかのユーザーのホーム（調べに行かなければならないもの）ではなく、この
	// リゾルバに与えられたホームディレクトリへの参照である。
	f.Add("~%d")
	f.Add("%d")
	f.Add("~%%d")
	f.Add("%%~")
	// NUL を挟むだけで、どのファイルも指しようのないパスができる。展開が
	// 「絶対パスになった」ことだけを見ていた頃は、これがそのまま Loader へ渡った。
	f.Add("a\x00b")

	resolver := Resolver{
		Home:   testHome,
		Root:   filepath.Join(testHome, ".ssh"),
		Tokens: map[byte]string{'d': testHome},
	}

	// '~user' の形式は、このエンジンが行わない passwd データベースの参照を必要と
	// するので、推測せずに拒否する。これはファズ本体のルールとしてではなく直接
	// 表明する。そこで表現すると、トークンとチルダが展開される順序を言い直すことに
	// なり、実装を言い直すテストは何ひとつ検査していないことに
	// なるからだ。
	for _, guessed := range []string{"~root/config", "~nobody", "~a/b", "a\x00b"} {
		if _, err := resolver.expandPattern(guessed); !errors.Is(err, ErrUnsupportedExpansion) {
			f.Fatalf("expandPattern(%q) = %v, want ErrUnsupportedExpansion", guessed, err)
		}
	}

	f.Fuzz(func(t *testing.T, argument string) {
		expanded, err := resolver.expandPattern(argument)
		if err != nil {
			if expanded != "" {
				t.Fatalf("expandPattern(%q) returned %q alongside %v", argument, expanded, err)
			}
			return
		}
		if !nativepath.Supported(expanded) {
			t.Fatalf("expandPattern(%q) = %q, which is not a supported absolute path", argument, expanded)
		}
		if cleaned := filepath.Clean(expanded); cleaned != expanded {
			t.Fatalf("expandPattern(%q) = %q, which is not cleaned (%q)", argument, expanded, cleaned)
		}
		// リゾルバがグロブするパスへ、未展開のパーセントトークンが生き残ってはならない。
		// リテラルのパーセントは "%%" のエスケープからしか生まれない。
		//
		// ここには意図してチルダに関するルールを置いていない。先頭のチルダは上の
		// 絶対性の検査ですでに排除されており、それ以外の位置のチルダはファイル名の中の
		// 普通の文字である。"%%~" はリテラルの名前 "%~" に展開され、OpenSSH もそれを
		// リテラルとして扱う。
		if strings.Contains(expanded, "%") && !strings.Contains(argument, "%%") {
			t.Fatalf("expandPattern(%q) = %q, which still contains an unexpanded token", argument, expanded)
		}
		again, againErr := resolver.expandPattern(argument)
		if againErr != nil || again != expanded {
			t.Fatalf("expandPattern(%q) is not deterministic: %q/%v then %q/%v", argument, expanded, err, again, againErr)
		}
	})
}
