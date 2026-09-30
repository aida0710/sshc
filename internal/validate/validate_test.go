package validate_test

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/validate"
)

func TestValidateAliasAcceptsOnlyTheSafeCharacterSet(t *testing.T) {
	accepted := []string{"bastion", "web-01", "db.internal", "a", "A1_b", strings.Repeat("h", 64)}
	for _, alias := range accepted {
		if err := validate.Alias(alias); err != nil {
			t.Errorf("ValidateAlias(%q) = %v, want nil", alias, err)
		}
	}

	rejected := []string{
		"",
		strings.Repeat("h", 65),
		"-oProxyCommand=touch /tmp/pwned",
		"--",
		"host name",
		"host;touch /tmp/pwned",
		`host"quote`,
		"host'quote",
		"host$(id)",
		"host`id`",
		"host\\escape",
		"host\nnewline",
		"host\ttab",
		"*",
		"!negated",
		"user@host",
		"%d",
		"../escape",
		"ホスト",
		".leading",
	}
	for _, alias := range rejected {
		if err := validate.Alias(alias); !errors.Is(err, validate.ErrUnsafeAlias) {
			t.Errorf("ValidateAlias(%q) = %v, want ErrUnsafeAlias", alias, err)
		}
	}
}

func TestValidateHostnameAcceptsNamesAndAddressesOnly(t *testing.T) {
	accepted := []string{
		"example.com", "bastion-1.internal", "203.0.113.10", "2001:db8::1",
		"::1", "2001:db8::", "::ffff:192.0.2.1", "host_name",
	}
	for _, host := range accepted {
		if err := validate.Hostname(host); err != nil {
			t.Errorf("ValidateHostname(%q) = %v, want nil", host, err)
		}
	}

	rejected := []string{"", "-p2222", "host name", "host;id", "[2001:db8::1]", "host/path", strings.Repeat("h", 256)}
	for _, host := range rejected {
		if err := validate.Hostname(host); !errors.Is(err, validate.ErrUnsafeHostname) {
			t.Errorf("ValidateHostname(%q) = %v, want ErrUnsafeHostname", host, err)
		}
	}
}

func TestValidatePortRejectsValuesOutsideTheTCPRange(t *testing.T) {
	for _, port := range []int{1, 22, 65535} {
		if err := validate.Port(port); err != nil {
			t.Errorf("ValidatePort(%d) = %v, want nil", port, err)
		}
	}
	for _, port := range []int{0, -1, 65536, 100000} {
		if err := validate.Port(port); !errors.Is(err, validate.ErrUnsafePort) {
			t.Errorf("ValidatePort(%d) = %v, want ErrUnsafePort", port, err)
		}
	}
}
