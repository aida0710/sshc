package remoteos

import "testing"

func TestParse(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"Linux\nID=amzn\nID_LIKE=fedora\nVERSION_ID=2023", "amazonlinux"},
		{"Linux\nID=\"amzn\"\nID_LIKE=\"centos rhel fedora\"\nVERSION_ID=2", "amazonlinux"},
		{"Linux\nID=fedora\n", "fedora"},
		{"Linux\nID=ubuntu\nID_LIKE=debian\n", "ubuntu"},
		{"Linux\nID=\"rhel\"\n", "redhat"},
		{"Linux\nID=rocky\nID_LIKE=\"rhel centos fedora\"", "rocky"},
		{"Linux\nID=custom\nID_LIKE='debian linux'", "debian"},
		{"Linux\nID=custom", "linux"}, {"Darwin\n", "macos"}, {"FreeBSD\n", "freebsd"},
		{"ID=$(touch /tmp/untrusted)\n", ""}, {"welcome to ubuntu", ""},
	} {
		if got := Parse(test.input); got != test.want {
			t.Errorf("Parse(%q)=%q want %q", test.input, got, test.want)
		}
	}
}
