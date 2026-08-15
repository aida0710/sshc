//go:build windows

package config

import (
	"errors"
	"testing"
)

func newWindowsDriveResolver() Resolver {
	return Resolver{
		Home:   `C:\Users\Tester`,
		Root:   `C:\Users\Tester\.ssh`,
		Tokens: map[byte]string{'d': `C:\Users\Tester`, 'u': "Tester", 'i': "1001"},
	}
}

func newWindowsUNCResolver() Resolver {
	return Resolver{
		Home:   `\\server\share\Tester`,
		Root:   `\\server\share\Tester\.ssh`,
		Tokens: map[byte]string{'d': `\\server\share\Tester`},
	}
}

// OpenSSH の Include はスラッシュで書かれる。その綴りは設定の構文であって、
// 行き先はこのファイルシステムのパスである。
func TestWindowsExpandPatternProducesNativeGlobs(t *testing.T) {
	resolver := newWindowsDriveResolver()
	for name, test := range map[string]struct{ argument, want string }{
		"relative resolves under the ssh directory": {"conf.d/*.conf", `C:\Users\Tester\.ssh\conf.d\*.conf`},
		"relative with backslashes":                 {`conf.d\*.conf`, `C:\Users\Tester\.ssh\conf.d\*.conf`},
		"tilde uses the home directory":             {"~/work/config", `C:\Users\Tester\work\config`},
		"bare tilde is the home directory":          {"~", `C:\Users\Tester`},
		"percent d is the home directory":           {"%d/.ssh/extra", `C:\Users\Tester\.ssh\extra`},
		"absolute drive stays absolute":             {`C:/tools/ssh_config`, `C:\tools\ssh_config`},
		"absolute drive in native spelling":         {`C:\tools\ssh_config`, `C:\tools\ssh_config`},
		"another drive stays on that drive":         {`D:\shared\ssh_config`, `D:\shared\ssh_config`},
		"ordinary UNC stays absolute":               {`\\server\share\ssh_config`, `\\server\share\ssh_config`},
		"parent segments are cleaned":               {"conf.d/../other.conf", `C:\Users\Tester\.ssh\other.conf`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolver.expandPattern(test.argument)
			if err != nil {
				t.Fatalf("expandPattern(%q) error = %v", test.argument, err)
			}
			if got != test.want {
				t.Fatalf("expandPattern(%q) = %q, want %q", test.argument, got, test.want)
			}
		})
	}
}

func TestWindowsExpandPatternWorksFromAUNCHome(t *testing.T) {
	resolver := newWindowsUNCResolver()
	for name, test := range map[string]struct{ argument, want string }{
		"relative under the share": {"conf.d/*.conf", `\\server\share\Tester\.ssh\conf.d\*.conf`},
		"tilde on the share":       {"~/work/config", `\\server\share\Tester\work\config`},
		"percent d on the share":   {"%d/.ssh/extra", `\\server\share\Tester\.ssh\extra`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolver.expandPattern(test.argument)
			if err != nil {
				t.Fatalf("expandPattern(%q) error = %v", test.argument, err)
			}
			if got != test.want {
				t.Fatalf("expandPattern(%q) = %q, want %q", test.argument, got, test.want)
			}
		})
	}
}

// 綴りが行き先を決めきれていないものは、推測せずに報告する。推測すれば、
// たまたま別のファイルが読まれる。
func TestWindowsExpandPatternRefusesAmbiguousAndUnsupportedSpellings(t *testing.T) {
	resolver := newWindowsDriveResolver()
	for _, argument := range []string{
		`C:config`,
		`\config`,
		`\\?\C:\config`,
		`\\.\C:\config`,
		`\??\C:\config`,
		`//?/C:/config`,
		"~other/config",
		"%h/config",
	} {
		if got, err := resolver.expandPattern(argument); !errors.Is(err, ErrUnsupportedExpansion) {
			t.Errorf("expandPattern(%q) = %q, %v; want ErrUnsupportedExpansion", argument, got, err)
		}
	}
}
