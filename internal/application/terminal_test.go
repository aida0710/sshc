package application

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/platform"
	"sshc/internal/storage"
)

func newTerminalService(t *testing.T) (*Service, *storage.Workspace) {
	t.Helper()
	workspace := newTestWorkspace(t)
	return NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader)), workspace
}

// 何も書かれていなければ home である。
//
// **エンジンの作業ディレクトリは継がない。** あれはエンジンを起こしたものが
// たまたま居た場所であり、利用者はそれを選んでいない。
func TestTheStartDirectoryDefaultsToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)

	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home %q", got, workspace.Home())
	}
}

// 書いた綴りのまま保存し、読むときに展開する。
//
// **home の綴りを設定に焼き付けない。** 焼き付けると、その設定は書いた機械で
// しか意味を持たない。
func TestTheStartDirectoryKeepsTheTildeAndResolvesItWhenRead(t *testing.T) {
	service, workspace := newTerminalService(t)
	work := filepath.Join(workspace.Home(), "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetTerminalStartDirectory("~/work"); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TerminalStartDirectory() != "~/work" {
		t.Fatalf("stored = %q, want the tilde kept", stored.TerminalStartDirectory())
	}
	if got := service.TerminalStartDirectory(); got != work {
		t.Fatalf("start directory = %q, want %q", got, work)
	}
}

// 通らない指定は保存のときに断る。
//
// 受け取っておいて次に端末を開いたときに失敗させると、設定画面と、失敗が
// 現れる場所が離れる。
func TestTheStartDirectoryIsRefusedWhenItCannotBeUsed(t *testing.T) {
	service, workspace := newTerminalService(t)
	file := filepath.Join(workspace.Home(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		given string
		want  error
	}{
		{name: "relative", given: "work", want: platform.ErrDirectoryRelative},
		{name: "another user", given: "~someone", want: platform.ErrDirectoryUser},
		{name: "missing", given: "~/nowhere", want: ErrStartDirectoryMissing},
		{name: "a file", given: file, want: ErrStartDirectoryNotADirectory},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.SetTerminalStartDirectory(test.given); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if got := service.TerminalStartDirectory(); got != workspace.Home() {
				t.Fatalf("the refusal changed the start directory to %q", got)
			}
		})
	}
}

// 保存は、利用者が選んでいないものを書かない。
//
// **既定を設定ファイルへ焼き付けない。** 焼き付けると、既定を変えた日に
// その人だけが取り残される——しかも黙って。読み取りが範囲外を既定へ戻すのは
// 読むたびの話であり、その結果を構造体に残してはならない。
func TestSavingDoesNotWriteSettingsNobodyChose(t *testing.T) {
	service, workspace := newTerminalService(t)

	// 二度保存する。一度目は節が無い状態から、二度目は自分が作った節の上から。
	for round := 0; round < 2; round++ {
		if _, err := service.SetTerminalStartDirectory("~"); err != nil {
			t.Fatal(err)
		}
	}

	contents, err := os.ReadFile(filepath.Join(workspace.Root(), "sshc", MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"maxSessions", "scrollbackBytes"} {
		if strings.Contains(string(contents), unwanted) {
			t.Fatalf("saving the start directory wrote %q into metadata:\n%s", unwanted, contents)
		}
	}
}

// 保存したあとに消えた場所は home へ倒す。
//
// **端末が開けなくなる方が悪い。** 開始位置は、開けることより弱い要求である。
func TestAStartDirectoryThatDisappearedFallsBackToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)
	work := filepath.Join(workspace.Home(), "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetTerminalStartDirectory("~/work"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}

	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home %q", got, workspace.Home())
	}
}

// 空文字は「書かれていない」に戻す。設定を消せなければ、一度指定した人は
// 二度と既定へ戻れない。
func TestClearingTheStartDirectoryReturnsToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)
	if _, err := service.SetTerminalStartDirectory("~"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetTerminalStartDirectory(""); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TerminalStartDirectory() != "" {
		t.Fatalf("stored = %q, want it cleared", stored.TerminalStartDirectory())
	}
	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home", got)
	}
}
