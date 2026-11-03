package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

// unknownDirectivesIn は、unknown_directive の診断を「ルートからの相対パス:行:キーワード」で並べる。
func unknownDirectivesIn(t *testing.T, graph *Graph) []string {
	t.Helper()
	found := []string{}
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code != DiagnosticUnknownDirective {
			continue
		}
		if diagnostic.Severity != SeverityWarning {
			t.Errorf("unknown_directive severity = %v, want warning", diagnostic.Severity)
		}
		relative, err := filepath.Rel(testRoot, diagnostic.Path)
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, fmt.Sprintf("%s:%d:%s", filepath.ToSlash(relative), diagnostic.Line, diagnostic.Detail))
	}
	return found
}

// OpenSSH は知らないキーワードの行がひとつでもあると設定全体を読まずに止まる。止まる行と
// 止まらない行の区別を、r540 の OpenSSH 10.2p1 の `ssh -G` で確かめた形で固定する。
// 実物との突き合わせは internal/effective の TestUnknownDirectivesMatchInstalledOpenSSH にある。
func TestResolveReportsTheLinesOpenSSHStopsAt(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "a misspelt keyword inside a host",
			files: map[string]string{"config": "Host typo-host\n\tHostNmae 192.0.2.77\n\tUser deploy\n"},
			want:  []string{"config:2:HostNmae"},
		},
		{
			name:  "a misspelt keyword before any host",
			files: map[string]string{"config": "Usr deploy\nHost a\n\tHostName 192.0.2.1\n"},
			want:  []string{"config:1:Usr"},
		},
		{
			// 表にある名前は、廃止（oDeprecated）・未対応（oUnsupported）・無視（oIgnore）でも止まらない。
			name:  "deprecated, unsupported and ignored keywords",
			files: map[string]string{"config": "Protocol 2\nUseRoaming no\nCipher blowfish\nRSAAuthentication yes\nCompressionLevel 6\n"},
			want:  []string{},
		},
		{
			name:  "keywords in any case and with an equals sign",
			files: map[string]string{"config": "HOSTNAME 192.0.2.1\nHostName=192.0.2.2\nUser = deploy\nHostNmae=192.0.2.3\n"},
			want:  []string{"config:4:HostNmae"},
		},
		{
			name:  "an ignore unknown before the line",
			files: map[string]string{"config": "IgnoreUnknown hostnmae\nHost a\n\tHOSTNMAE 192.0.2.1\n"},
			want:  []string{},
		},
		{
			name:  "an ignore unknown written with an equals sign and patterns",
			files: map[string]string{"config": "IgnoreUnknown=Foo,Host*\nHost a\n\tHostNmae 192.0.2.1\n"},
			want:  []string{},
		},
		{
			// ssh は IgnoreUnknown を読んだ後ろの行にしか使わない。
			name:  "an ignore unknown after the line",
			files: map[string]string{"config": "HostNmae 192.0.2.1\nIgnoreUnknown HostNmae\n"},
			want:  []string{"config:1:HostNmae"},
		},
		{
			// 最初に読んだ値だけが効く。2つ目の IgnoreUnknown は読み飛ばされる。
			name:  "a second ignore unknown",
			files: map[string]string{"config": "IgnoreUnknown Foo\nIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n"},
			want:  []string{"config:4:HostNmae"},
		},
		{
			name:  "a negated pattern in ignore unknown",
			files: map[string]string{"config": "IgnoreUnknown *,!hostnmae\nHost a\n\tHostNmae 192.0.2.1\n\tUsr x\n"},
			want:  []string{"config:3:HostNmae"},
		},
		{
			name:  "an ignore unknown inside host star and match all",
			files: map[string]string{"config": "Host *\n\tIgnoreUnknown Foo*\nMatch all\n\tIgnoreUnknown Bar\nHost a\n\tFooBar yes\n\tBar yes\n"},
			want:  []string{"config:7:Bar"},
		},
		{
			// 接続先が probe なら IgnoreUnknown が効くが、ほかの接続先では効かずに止まる。
			name:  "an ignore unknown that only some destinations read",
			files: map[string]string{"config": "Host probe\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n"},
			want:  []string{"config:4:HostNmae"},
		},
		{
			// probe では Foo が最初の値になり、HostNmae で止まる。
			name:  "a conditional ignore unknown read before one every destination reads",
			files: map[string]string{"config": "Host probe\n\tIgnoreUnknown Foo\nHost *\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n"},
			want:  []string{"config:6:HostNmae"},
		},
		{
			name:  "a host that also excludes a name",
			files: map[string]string{"config": "Host * !probe\n\tIgnoreUnknown HostNmae\nHost a\n\tHostNmae 192.0.2.1\n"},
			want:  []string{"config:4:HostNmae"},
		},
		{
			name:  "an unknown keyword inside a match that matches nothing",
			files: map[string]string{"config": "Match host nothing.invalid\n\tHostNmae 192.0.2.1\n"},
			want:  []string{"config:2:HostNmae"},
		},
		{
			name: "an unknown keyword in an included file",
			files: map[string]string{
				"config":           "Include conf.d/*.conf\nHost after\n\tHostName 192.0.2.9\n",
				"conf.d/work.conf": "Host work\n\tHostNmae 192.0.2.1\n",
			},
			want: []string{"conf.d/work.conf:2:HostNmae"},
		},
		{
			// 取り込んだファイルの先頭の行と Host * は、取り込み元の Include 行がどこでも読まれる
			// 位置にあれば、どの接続先でも読まれる。
			name: "an ignore unknown in an included file",
			files: map[string]string{
				"config":         "Include conf.d/*.conf\nHost a\n\tHostNmae 192.0.2.1\n",
				"conf.d/10.conf": "Host *\n\tIgnoreUnknown HostNmae\n",
			},
			want: []string{},
		},
		{
			// 一致しない接続先では、取り込んだファイルの Host * も一致しない（SSHCONF_NEVERMATCH）。
			name: "an ignore unknown in a file included from inside a host",
			files: map[string]string{
				"config":         "Host other\n\tInclude conf.d/*.conf\nHost a\n\tHostNmae 192.0.2.1\n",
				"conf.d/10.conf": "Host *\n\tIgnoreUnknown HostNmae\n",
			},
			want: []string{"config:4:HostNmae"},
		},
		{
			name: "an unknown keyword in a file included from inside a host",
			files: map[string]string{
				"config":         "Host other\n\tInclude conf.d/*.conf\n",
				"conf.d/10.conf": "Usr deploy\n",
			},
			want: []string{"conf.d/10.conf:1:Usr"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := make(map[string]string, len(test.files))
			for relative, contents := range test.files {
				files[filepath.Join(testRoot, filepath.FromSlash(relative))] = contents
			}
			graph, err := resolverFor(files).Resolve(testConfig)
			if err != nil {
				t.Fatal(err)
			}
			if got := unknownDirectivesIn(t, graph); !slices.Equal(got, test.want) {
				t.Errorf("unknown directives = %q, want %q", got, test.want)
			}
		})
	}
}

// 診断の文は、利用者が書いたとおりのキーワードを出す。OpenSSH のエラーは小文字に
// するが、打ち間違いを探すのは書いた本人である。
func TestUnknownDirectiveNamesTheKeywordAsWritten(t *testing.T) {
	graph, err := resolverFor(map[string]string{testConfig: "Host a\n\tHostNmae 192.0.2.77\n"}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := requireDiagnostic(t, graph, DiagnosticUnknownDirective)
	if diagnostic.Path != testConfig || diagnostic.Line != 2 || diagnostic.Detail != "HostNmae" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

// UseKeychain は macOS の ssh だけが受け付ける。ほかの OS の OpenSSH は止まる。
func TestOnlyMacOSAcceptsUseKeychain(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "linux": false, "windows": false, "freebsd": false} {
		if got := openSSHKnows("UseKeychain", goos); got != want {
			t.Errorf("openSSHKnows(UseKeychain, %s) = %v, want %v", goos, got, want)
		}
	}
}

// 表の作り直しで読み方を誤ると、ssh が読む行まで知らないと言い出す。ふつうの設定に出る
// キーワードと、廃止・未対応・無視の名前が表にあることを確かめる。
func TestTheKeywordTableHoldsOpenSSHsNames(t *testing.T) {
	for _, name := range []string{
		"host", "match", "include", "ignoreunknown", "hostname", "user", "port", "identityfile",
		"proxyjump", "proxycommand", "serveraliveinterval",
		"useroaming", "protocol", "rsaauthentication", "gssapiauthentication", "pkcs11provider",
	} {
		if !openSSHKnows(name, "linux") {
			t.Errorf("%s is missing from the keyword table", name)
		}
	}
	if !slices.IsSorted(openSSHKeywordNames) {
		t.Error("the keyword table is not sorted; regenerate it with scripts/openssh/generate-config-keywords.py")
	}
}
