package application

import (
	"path/filepath"
	"testing"

	"sshc/internal/effective"
)

// testFacts は、トークン展開に要る事実を固定する。本物のホームやアカウント名を
// 読むと、テストが走るマシンによって結果が変わる。
func testFacts() effective.LocalFacts {
	return effective.LocalFacts{User: "tester", Home: testHome, Hostname: "fixture.local", UID: "501"}
}

const effectiveFiles = `Include conf.d/*.conf
Host bastion
	User ops
	IdentityFile ~/.ssh/id_a
	IdentityFile ~/.ssh/id_b
Match host bastion
	User match-user
Host *
	User fallback
	ServerAliveInterval 30
`

func TestComputeEffectiveTakesTheFirstValueAndKeepsItsSource(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config":               effectiveFiles,
		"conf.d/10-first.conf": "Host bastion\n\tPort 2200\n",
	})

	effective := ComputeEffective(graph, testRoot, "bastion", testFacts())
	// 但し書きは無くなった。この値は説明ではなく答えである。答えられなかった
	// ことを言う notice が出ていないことを確かめる——重複 alias のような、
	// 答えは確定しているが読み手に伝えたい印は出てよい。
	for _, notice := range effective.Notices {
		if notice.Code == NoticeExplainedValuesOnly || notice.Code == NoticeMatchExecRefused {
			t.Fatalf("a configuration this engine can answer refused: %#v", notice)
		}
	}
	want := []struct {
		keyword string
		value   string
		path    string
		line    int
	}{
		{"IdentityFile", "~/.ssh/id_a", "config", 4},
		{"IdentityFile", "~/.ssh/id_b", "config", 5},
		{"Port", "2200", "conf.d/10-first.conf", 2},
		{"ServerAliveInterval", "30", "config", 10},
		{"User", "ops", "config", 3},
	}
	if len(effective.Entries) != len(want) {
		t.Fatalf("entries = %#v", effective.Entries)
	}
	for index, expected := range want {
		entry := effective.Entries[index]
		if entry.Keyword != expected.keyword || entry.Values[0] != expected.value {
			t.Fatalf("entry[%d] = %#v, want %q %q", index, entry, expected.keyword, expected.value)
		}
		if entry.Source.Path != expected.path || entry.Source.Line != expected.line {
			t.Fatalf("entry[%d] source = %#v, want %q line %d", index, entry.Source, expected.path, expected.line)
		}
	}

	codes := map[string]bool{}
	for _, notice := range effective.Notices {
		codes[notice.Code] = true
	}
	// 二つのブロックが bastion を名乗っているので、その印は残る。答えは
	// 確定しているが、書いた本人には見えていないからである。
	if !codes[NoticeDuplicateAlias] {
		t.Fatalf("notices = %#v, want a duplicate_alias", effective.Notices)
	}
	// 権威に委ねるための但し書きは、もう出ない。Match ブロックは評価されるので
	// 「評価されない」という警告も出ない。
	for _, gone := range []string{NoticeExplainedValuesOnly, NoticeComplexExternalRule, NoticeMatchBlock} {
		if codes[gone] {
			t.Errorf("notice %q outlived the deferral to ssh -G: %#v", gone, effective.Notices)
		}
	}
}

func TestComputeEffectiveIgnoresBlocksThatDoNotMatch(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config": "Host other\n\tUser other-user\nHost !bastion *\n\tUser negated\n",
	})
	effective := ComputeEffective(graph, testRoot, "bastion", testFacts())
	if len(effective.Entries) != 0 {
		t.Fatalf("entries = %#v", effective.Entries)
	}
}

func TestDiffEffectiveReportsAddedChangedAndRemovedValues(t *testing.T) {
	before := Effective{Alias: "build01", Entries: []EffectiveEntry{
		{Keyword: "User", Values: []string{"ops"}, Source: Source{Path: "config", Line: 3}},
		{Keyword: "Port", Values: []string{"22"}, Source: Source{Path: "config", Line: 4}},
	}}
	after := Effective{Alias: "build01", Entries: []EffectiveEntry{
		{Keyword: "User", Values: []string{"ops"}, Source: Source{Path: "config", Line: 3}},
		{Keyword: "Port", Values: []string{"2222"}, Source: Source{Path: "groups.sshc.conf", Line: 5}},
		{Keyword: "ServerAliveInterval", Values: []string{"30"}, Source: Source{Path: "groups.sshc.conf", Line: 6}},
	}}

	diff := DiffEffective(before, after)
	if diff.Alias != "build01" || len(diff.Changes) != 2 {
		t.Fatalf("diff = %#v", diff)
	}
	if diff.Changes[0].Keyword != "Port" || diff.Changes[0].Before[0] != "22" || diff.Changes[0].After[0] != "2222" {
		t.Fatalf("port change = %#v", diff.Changes[0])
	}
	if diff.Changes[0].AfterSources[0].Path != "groups.sshc.conf" {
		t.Fatalf("port source = %#v", diff.Changes[0].AfterSources)
	}
	if diff.Changes[1].Keyword != "ServerAliveInterval" || len(diff.Changes[1].Before) != 0 {
		t.Fatalf("added change = %#v", diff.Changes[1])
	}
}

// TestDiffEffectiveIgnoresALineShiftButNotARealMove は、Source の
// 変化として数えるものを固定する。ファイルへの行挿入はその下の
// すべての値を押し下げるが、単に移動しただけの不変の値は
// ユーザーが編集したものではない。別のファイルや別のブロックへ移動した値はそうである。
func TestDiffEffectiveIgnoresALineShiftButNotARealMove(t *testing.T) {
	// Source.Path は UI が持ち回るスラッシュ区切りの識別子、Source.Absolute は
	// リゾルバが返したこのファイルシステムのパスである。
	configPath := filepath.Join(testRoot, "config")
	groupsPath := filepath.Join(testRoot, "groups.sshc.conf")

	before := Effective{Alias: "nas", Entries: []EffectiveEntry{
		{Keyword: "ServerAliveInterval", Values: []string{"30"}, Source: Source{Path: "config", Absolute: configPath, Line: 10, Condition: "Host *"}},
	}}

	shifted := Effective{Alias: "nas", Entries: []EffectiveEntry{
		{Keyword: "ServerAliveInterval", Values: []string{"30"}, Source: Source{Path: "config", Absolute: configPath, Line: 11, Condition: "Host *"}},
	}}
	if diff := DiffEffective(before, shifted); len(diff.Changes) != 0 {
		t.Fatalf("a pure line shift was reported as a change: %#v", diff.Changes)
	}

	movedFile := Effective{Alias: "nas", Entries: []EffectiveEntry{
		{Keyword: "ServerAliveInterval", Values: []string{"30"}, Source: Source{Path: "groups.sshc.conf", Absolute: groupsPath, Line: 7, Condition: "Host nas"}},
	}}
	if diff := DiffEffective(before, movedFile); len(diff.Changes) != 1 {
		t.Fatalf("a move to another file was not reported: %#v", diff.Changes)
	}

	movedBlock := Effective{Alias: "nas", Entries: []EffectiveEntry{
		{Keyword: "ServerAliveInterval", Values: []string{"30"}, Source: Source{Path: "config", Absolute: configPath, Line: 10, Condition: "Host nas"}},
	}}
	if diff := DiffEffective(before, movedBlock); len(diff.Changes) != 1 {
		t.Fatalf("a move to another block was not reported: %#v", diff.Changes)
	}
}

// host 詳細の Effective タブは、ユーザーが「実際には何を得るのか?」と
// 問う場所である。2 つのファイルが同じ alias を主張しているのは、
// 画面上のブロックが言うことと答えが異なる唯一の状況であり、このタブが最も
// 言及すべき状況だが、connections tree はそれを示すのに、これまでは何も言っていなかった。
func TestComputeEffectiveReportsAnAliasClaimedByTwoFiles(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config":              "Include conf.d/*.conf\n\nHost nas\n\tUser aida\n",
		"conf.d/10-home.conf": "Host nas\n\tUser someone-else\n",
	})

	effective := ComputeEffective(graph, testRoot, "nas", testFacts())
	found := false
	for _, notice := range effective.Notices {
		if notice.Code == NoticeDuplicateAlias {
			found = true
		}
	}
	if !found {
		t.Errorf("notices = %#v, want a duplicate_alias", effective.Notices)
	}
}

func TestComputeEffectiveDoesNotCallOneBlockADuplicate(t *testing.T) {
	graph := newTestGraph(t, map[string]string{"config": "Host nas\n\tUser aida\n"})

	for _, notice := range ComputeEffective(graph, testRoot, "nas", testFacts()).Notices {
		if notice.Code == NoticeDuplicateAlias {
			t.Errorf("a single block was reported as a duplicate: %#v", notice)
		}
	}
}
