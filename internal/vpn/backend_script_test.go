package vpn

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// stageBackendScript は、backend の手順を、読み込む共通の手順と一緒に directory へ置く。
//
// 手順は backend.sh の名前で置く。共通の手順は `$backend_directory` から読むので、
// 手順を読み込む script は、先に backend_directory を directory にする。
func stageBackendScript(t *testing.T, directory string, name BackendName) {
	t.Helper()
	for source, staged := range map[string]string{
		"container/backend-" + string(name) + ".sh": "backend.sh",
		"container/strongswan.sh":                   "strongswan.sh",
	} {
		contents, err := container.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, staged), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// agentBackendBranch は、agent.sh が backend の手順を読み込む case の枝である。
var agentBackendBranch = regexp.MustCompile(`(?m)^case "\$backend" in\n([a-z0-9_ |]+)\)\n`)

// agent.sh は、backends の表にある方式の手順をすべて読み込み、それ以外は読み込まない。
//
// 方式を足すたびに、枝をひとつにまとめたまま名前を足す。表と食い違うと、足した方式が
// コンテナの中で「対応していない方式です」で止まる。
func TestTheAgentLoadsEveryBackendOfTheTable(t *testing.T) {
	agent, err := container.ReadFile("container/agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	branches := agentBackendBranch.FindAllSubmatch(agent, -1)
	if len(branches) != 1 {
		t.Fatalf("agent.sh の backend の枝がひとつではない: %d", len(branches))
	}
	var loaded []string
	for _, name := range strings.Split(string(branches[0][1]), "|") {
		loaded = append(loaded, strings.TrimSpace(name))
	}
	var known []string
	for name := range backends {
		known = append(known, string(name))
	}
	sort.Strings(loaded)
	sort.Strings(known)
	if !slices.Equal(loaded, known) {
		t.Fatalf("agent.sh が読み込む方式 = %v, backends の表 = %v", loaded, known)
	}
}
