package application

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"sshc/internal/randomid"
	"sshc/internal/storage"
)

// このマシンの ID。マシンごとのファイル（machine_settings.go、machine_state.go）の名前に使う。
//
// ID のファイルは同期しない（remotesync の neverTravels）。運ぶと、別のマシンが同じ ID を名乗り、
// 使うプリセットとポートと今の状態を取り合う。同期で ~/.ssh を作り直したマシンは新しい ID を
// 作り、別のマシンとして並ぶ。
const (
	MachineIDFileName = "machine-id"
	// MachineIDPathRelative は、ワークスペースからのスラッシュ区切りの相対パスである。
	// remotesync は上の層の application を参照しないので同じ文字列を持ち、app のテストが照合する。
	MachineIDPathRelative = storage.StateDirectoryName + "/" + MachineIDFileName
	// machineIDTemporaryPrefix は、書くときの一時ファイルの名前の先頭。".sshc-" で始め、書きかけが
	// 残っても同期で運ばないようにする。
	machineIDTemporaryPrefix = ".sshc-machine-id-"
)

// machineIDPattern は、randomid.UnusedID が作る ID の形（16バイトの16進）である。
var machineIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var ErrMachineID = errors.New("the machine id file is unreadable")

func (s *Service) machineIDPath() string {
	return filepath.Join(s.workspace.StateDir(), MachineIDFileName)
}

// ensureMachineID は、このマシンの ID を読み、無ければ作って書く。
func (s *Service) ensureMachineID(random io.Reader) (string, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.machineIDPath())
	if err == nil {
		id := strings.TrimSpace(string(contents))
		if !machineIDPattern.MatchString(id) {
			return "", fmt.Errorf("%w: %s", ErrMachineID, s.machineIDPath())
		}
		return id, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	id, err := randomid.UnusedID(random, func(candidate string) (bool, error) {
		_, exists, err := s.readFile(s.machineSettingsPath(candidate))
		return exists, err
	})
	if err != nil {
		return "", err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return "", err
	}
	// このマシンだけのファイルなので、変更の履歴には残さない。
	if err := storage.WriteAtomicFile(s.workspace.FileSystem(), s.machineIDPath(), machineIDTemporaryPrefix, storage.FilePermission, []byte(id+"\n")); err != nil {
		return "", err
	}
	return id, nil
}

// MachineID は、このマシンの ID を返す。PrepareMachine の前と、それを呼ばない構成（テストなど）では
// 空で、そのときはどの設定も標準のプリセットで読む。
func (s *Service) MachineID() string {
	return s.machineID
}
