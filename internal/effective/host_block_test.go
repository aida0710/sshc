package effective

import (
	"testing"

	"sshc/internal/config"
)

// Host 行は OpenSSH と同じ規則で読む: 否定 pattern が 1 個でも当たれば行全体を
// 却下し、そうでなければ肯定 pattern のどれかが当たれば適用する。
func TestHostBlockAppliesFollowsOpenSSHPatternRules(t *testing.T) {
	tests := []struct {
		patterns  string
		candidate string
		want      bool
	}{
		{"bastion", "bastion", true},
		{"bastion", "Bastion", false},
		{"*.internal", "db.internal", true},
		{"*.internal", "internal", false},
		{"web?", "web1", true},
		{"web?", "web12", false},
		{"*", "anything", true},
		{"!secret *.internal", "secret", false},
		{"!secret *.internal", "db.internal", true},
		{"a* !ab", "ab", false},
		{"a* !ab", "ac", true},
	}
	for _, test := range tests {
		header := config.Parse([]byte("Host " + test.patterns + "\n"))
		block := header.Blocks()[1]
		if _, got := blockApplies(block, test.candidate); got != test.want {
			t.Errorf("blockApplies(%q, %q) = %v, want %v", test.patterns, test.candidate, got, test.want)
		}
	}
}
