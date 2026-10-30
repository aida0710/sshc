package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"sshc/internal/storage"
	"sshc/internal/strictjson"
)

// マシンの今の状態（転送の処理を止めているか、画面の並びと開いているもの）。同期するが、その
// マシンにだけ効き、そのマシンのエンジンだけが書く（machines/<ID>.state.json）。設定の画面には
// 出さない。
//
// 画面の並びはクリックのたびに変わる。そのたびに書くと、同期がそのたびにスナップショットを送る
// ので、エンジンがメモリに持ち、最初の変更から machineStateFlushDelay 待ってからまとめて書く。
// 転送の処理を止めたことは、書く前にエンジンが落ちても止めたままにするため、すぐに書く。
//
// 変更の履歴（storage の journal）には残さない。履歴は利用者が戻すためのもので、数十秒ごとの
// 今の状態で埋めると、戻したい変更が見つからなくなる。
const (
	machineStateSchemaVersion = 1
	// machineStateFlushDelay は、画面の並びの変更をまとめて書くまでの時間である。今の状態が
	// 別のマシンで要るのは、作り直したときと見比べるときだけなので、数十秒遅れてかまわない。
	machineStateFlushDelay = 30 * time.Second
	// maxInterfaceStateBytes は、画面の並び1つの大きさの上限である。SFTP のタブを数十開いても
	// 収まり、壊れた画面が大きな値を書き続けても同期を重くしない大きさにする。
	maxInterfaceStateBytes = 64 << 10
	// machineStateTemporaryPrefix は、書き換えに使う一時ファイルの名前の先頭。storage の一時ファイルと
	// 同じ ".sshc-" で始め、書きかけが残っても同期で運ばないようにする。
	machineStateTemporaryPrefix = ".sshc-machine-state-"
)

// interfaceStateKeys は、画面の並びとして預かる名前である。値の形は画面が決め、エンジンは
// 中を読まない（web/src/settings/machineState.ts）。
var interfaceStateKeys = map[string]bool{
	"runTarget":                    true,
	"sftpPanes":                    true,
	"sftpSplitRatio":               true,
	"sftpQueueView":                true,
	"collapsedTerminalWorkspaces":  true,
	"showHiddenTerminalWorkspaces": true,
}

var ErrMachineState = errors.New("machine state is invalid")

// MachineFacts は、そのマシンのエンジンが最後に起動したときの記録である。マシンの一覧に出す。
type MachineFacts struct {
	OS        string `json:"os,omitempty"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
}

// MachineState は、1台のマシンの今の状態である。
type MachineState struct {
	Facts MachineFacts `json:"facts"`
	// TransfersStopped は、SFTP の転送の処理を止めているかである。
	TransfersStopped bool `json:"transfersStopped,omitempty"`
	// Interface は画面の並びと開いているもの（interfaceStateKeys）で、値は画面が書いた文字列のまま。
	Interface map[string]string `json:"interface,omitempty"`
}

type machineStateDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	MachineState
}

func validateMachineState(state MachineState) error {
	size := 0
	for key, value := range state.Interface {
		if !interfaceStateKeys[key] {
			return fmt.Errorf("%w: unknown interface state %q", ErrMachineState, key)
		}
		size += len(value)
	}
	if size > maxInterfaceStateBytes {
		return fmt.Errorf("%w: interface state too large", ErrMachineState)
	}
	return nil
}

func decodeMachineState(contents []byte) (MachineState, error) {
	var document machineStateDocument
	if err := strictjson.Decode(contents, &document); err != nil {
		return MachineState{}, fmt.Errorf("%w: %w", ErrMachineState, err)
	}
	if document.SchemaVersion != machineStateSchemaVersion {
		return MachineState{}, fmt.Errorf("%w: schema version %d", ErrMachineState, document.SchemaVersion)
	}
	if err := validateMachineState(document.MachineState); err != nil {
		return MachineState{}, err
	}
	return document.MachineState, nil
}

func encodeMachineState(state MachineState) ([]byte, error) {
	if err := validateMachineState(state); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(machineStateDocument{SchemaVersion: machineStateSchemaVersion, MachineState: state}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func (s *Service) machineStatePath(id string) string {
	return filepath.Join(s.machinesDirectory(), id+".state.json")
}

// machineState は、ID のマシンの今の状態をディスクから読む。読めなければ空の状態とする。
func (s *Service) machineState(id string) MachineState {
	contents, exists, err := s.readFile(s.machineStatePath(id))
	if err != nil || !exists {
		return MachineState{}
	}
	state, err := decodeMachineState(contents)
	if err != nil {
		return MachineState{}
	}
	return state
}

// machineStateMemory は、このマシンの今の状態のうち、まだ書いていないものを持つ。
type machineStateMemory struct {
	mutex sync.Mutex
	// state は今の状態である。loaded が false の間はディスクから読んでいない。
	state  MachineState
	loaded bool
	// pending は、まとめて書くのを待っている変更があることを表す。
	pending bool
	timer   *time.Timer
}

// currentMachineState は、メモリの状態を返す。初めて呼ばれたときにディスクから読む。呼ぶ側が
// mutex を持つ。
func (s *Service) currentMachineState() MachineState {
	memory := &s.stateMemory
	if !memory.loaded {
		memory.state = s.machineState(s.machineID)
		memory.loaded = true
	}
	return memory.state
}

// ThisMachineState は、このマシンの今の状態を、まだ書いていない変更も含めて返す。
func (s *Service) ThisMachineState() MachineState {
	if s.machineID == "" {
		return MachineState{}
	}
	s.stateMemory.mutex.Lock()
	defer s.stateMemory.mutex.Unlock()
	return cloneMachineState(s.currentMachineState())
}

// SetInterfaceState は、画面の並びを変える。空の値はその名前を消す。書くのは
// machineStateFlushDelay のあとでまとめて行う。
func (s *Service) SetInterfaceState(changes map[string]string) error {
	if s.machineID == "" {
		return ErrUnknownMachine
	}
	s.stateMemory.mutex.Lock()
	defer s.stateMemory.mutex.Unlock()
	next := cloneMachineState(s.currentMachineState())
	if next.Interface == nil {
		next.Interface = map[string]string{}
	}
	for key, value := range changes {
		if value == "" {
			delete(next.Interface, key)
			continue
		}
		next.Interface[key] = value
	}
	if err := validateMachineState(next); err != nil {
		return err
	}
	s.stateMemory.state = next
	s.scheduleMachineStateFlush()
	return nil
}

// SetTransfersStopped は、転送の処理を止めているかを変え、すぐに書く。
func (s *Service) SetTransfersStopped(stopped bool) error {
	return s.updateMachineStateNow(func(state *MachineState) { state.TransfersStopped = stopped })
}

// recordMachineStart は、エンジンが起動したことを今の状態に書く。マシンの一覧の「最後の起動」は
// これを出す。時計で書き直すことはしない（書くたびに同期が走る）。
func (s *Service) recordMachineStart(facts MachineFacts) error {
	return s.updateMachineStateNow(func(state *MachineState) { state.Facts = facts })
}

func (s *Service) updateMachineStateNow(change func(*MachineState)) error {
	if s.machineID == "" {
		return ErrUnknownMachine
	}
	s.stateMemory.mutex.Lock()
	defer s.stateMemory.mutex.Unlock()
	next := cloneMachineState(s.currentMachineState())
	change(&next)
	s.stateMemory.state = next
	return s.writeMachineState()
}

// scheduleMachineStateFlush は、まとめて書く時計を掛ける。掛かっていれば掛け直さない。最初の
// 変更から数えるので、変え続けても書くのが遅れ続けることはない。呼ぶ側が mutex を持つ。
func (s *Service) scheduleMachineStateFlush() {
	memory := &s.stateMemory
	memory.pending = true
	if memory.timer != nil {
		return
	}
	memory.timer = time.AfterFunc(machineStateFlushDelay, func() {
		memory.mutex.Lock()
		defer memory.mutex.Unlock()
		memory.timer = nil
		if memory.pending {
			// 書けなかった変更は、次の変更か、エンジンの停止（FlushMachineState）でまた書く。
			_ = s.writeMachineState()
		}
	})
}

// FlushMachineState は、まだ書いていない今の状態を書く。エンジンを止める前に呼ぶ。
func (s *Service) FlushMachineState() error {
	if s.machineID == "" {
		return nil
	}
	s.stateMemory.mutex.Lock()
	defer s.stateMemory.mutex.Unlock()
	if s.stateMemory.timer != nil {
		s.stateMemory.timer.Stop()
		s.stateMemory.timer = nil
	}
	if !s.stateMemory.pending {
		return nil
	}
	return s.writeMachineState()
}

// writeMachineState は、メモリの状態をファイルに書き、書いたことを OnMachineStateWritten で足した
// 関数に知らせる。呼ぶ側が mutex を持つ。
func (s *Service) writeMachineState() error {
	contents, err := encodeMachineState(s.stateMemory.state)
	if err != nil {
		return err
	}
	path := s.machineStatePath(s.machineID)
	current, exists, err := s.readFile(path)
	if err != nil {
		return err
	}
	if exists && string(current) == string(contents) {
		s.stateMemory.pending = false
		return nil
	}
	if err := s.workspace.EnsureDirectory(s.machinesDirectory()); err != nil {
		return err
	}
	if err := storage.WriteAtomicFile(s.workspace.FileSystem(), path, machineStateTemporaryPrefix, storage.FilePermission, contents); err != nil {
		return err
	}
	s.stateMemory.pending = false
	for _, listener := range s.stateListeners {
		listener()
	}
	return nil
}

// OnMachineStateWritten は、このマシンの今の状態を書いたときに呼ぶ関数を足す。今の状態は変更の
// 履歴を通らないので、同期に送るものができたことを、エンジンはこれで知る。
func (s *Service) OnMachineStateWritten(listener func()) {
	s.stateListeners = append(s.stateListeners, listener)
}

func cloneMachineState(state MachineState) MachineState {
	clone := state
	if state.Interface != nil {
		clone.Interface = make(map[string]string, len(state.Interface))
		for key, value := range state.Interface {
			clone.Interface[key] = value
		}
	}
	return clone
}
