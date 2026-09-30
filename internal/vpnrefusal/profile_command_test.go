package vpnrefusal

import (
	"runtime"
	"testing"
)

// 案内するコマンドは、空白や引用符を含む名前でもそのまま貼り付けて動く。
func TestAProfileCommandCanBePastedForAnyName(t *testing.T) {
	if got, want := ProfileCommand("logs", "lab"), "sshc vpn logs lab"; got != want {
		t.Fatalf("ProfileCommand = %q, want %q", got, want)
	}
	if got, want := ProfileCommand("logs", "Bob's VPN"), "sshc vpn logs "+shellWord(runtime.GOOS, "Bob's VPN"); got != want {
		t.Fatalf("ProfileCommand = %q, want %q", got, want)
	}
}

// 名前は、sshcエンジンと CLI が動くマシンのシェルが1語と読む形で引用する。
func TestANameIsQuotedForTheShellOfTheMachine(t *testing.T) {
	for _, test := range []struct{ goos, name, want string }{
		{"linux", "lab", "lab"},
		{"linux", "研究室 VPN", "'研究室 VPN'"},
		{"darwin", "Bob's VPN", `'Bob'"'"'s VPN'`},
		{"windows", "lab", "lab"},
		// PowerShell と cmd.exe のどちらでも1語になり、pages の案内と同じ二重引用符にする。
		{"windows", "研究室 VPN", `"研究室 VPN"`},
		{"windows", "Bob's VPN", `"Bob's VPN"`},
		// 二重引用符の中でも展開される字を含む名前は、PowerShell の一重引用符で囲む。
		{"windows", `the "lab" VPN`, `'the "lab" VPN'`},
		{"windows", "Bob's $HOME VPN", `'Bob''s $HOME VPN'`},
		{"windows", "100% VPN", `'100% VPN'`},
	} {
		if got := shellWord(test.goos, test.name); got != test.want {
			t.Errorf("%s: shellWord(%q) = %q, want %q", test.goos, test.name, got, test.want)
		}
	}
}
