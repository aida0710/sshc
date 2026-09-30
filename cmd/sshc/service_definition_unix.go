//go:build !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"sshc/internal/storage"
)

// serviceExecutablePlaceholder は、定義の形だけを比べるときに、実行ファイルの代わりに
// 書くパスである。systemd の unit でも launchd の plist でもエスケープされない文字だけで
// できている。
const serviceExecutablePlaceholder = "/sshc-service-executable"

// serviceDefinitionFile は、`sshc service install` が書く定義ファイル（systemd の unit、
// launchd の plist）1 つを表す。install、disable、update の再起動が、書き換えの前後で
// 内容が変わっていないことと、登録した実行ファイルと一致することを同じ手順で確かめる。
type serviceDefinitionFile struct {
	files storage.FileSystem
	path  string
	// marker は、sshc が書いた定義の先頭にある印。無ければ手書きの定義とみなして触らない。
	marker string
	// name は、エラーでこの定義を指す名前。
	name string
	// render は、実行ファイルを登録した定義の内容を返す。
	render func(executable string) (string, error)
}

type serviceDefinitionSnapshot struct {
	state    serviceState
	contents []byte
}

func (definition serviceDefinitionFile) readSnapshot() (serviceDefinitionSnapshot, error) {
	contents, err := definition.files.ReadFile(definition.path)
	if errors.Is(err, os.ErrNotExist) {
		return serviceDefinitionSnapshot{state: serviceAbsent}, nil
	}
	if err != nil {
		return serviceDefinitionSnapshot{state: serviceAbsent}, fmt.Errorf("read %s: %w", definition.path, err)
	}
	if !bytes.HasPrefix(contents, []byte(definition.marker)) {
		return serviceDefinitionSnapshot{state: serviceUnmanaged, contents: contents}, nil
	}
	return serviceDefinitionSnapshot{state: serviceInactive, contents: contents}, nil
}

// ensureUnchanged は、操作の途中で定義が書き換えられていれば、それを上書きも削除も
// せずに失敗する。
func (definition serviceDefinitionFile) ensureUnchanged(expected serviceDefinitionSnapshot) error {
	actual, err := definition.readSnapshot()
	if err != nil {
		return err
	}
	if actual.state != expected.state || !bytes.Equal(actual.contents, expected.contents) {
		return fmt.Errorf("%s changed during the operation; it was left in place", definition.name)
	}
	return nil
}

// matches は、定義が executable を登録した sshc の定義と完全に一致するかを返す。
func (definition serviceDefinitionFile) matches(executable string) (bool, error) {
	expected, err := definition.render(executable)
	if err != nil {
		return false, err
	}
	contents, err := definition.files.ReadFile(definition.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", definition.path, err)
	}
	return bytes.Equal(contents, []byte(expected)), nil
}

// isOutdated は、sshc の印がある定義が、登録した実行ファイルのパスのほかで、今の sshc の
// 書く定義と違うかを返す。定義の中身を変えた sshc へ更新しても、既存の定義は
// `sshc service install` を実行し直すまで以前の版のままである。印が無い定義と、定義が
// 無いときは false を返す。
//
// 実行ファイルのパスは定義に 1 か所だけ入るので、仮のパスで書いた定義をそのパスの
// 前後に分け、実際の定義の先頭と末尾がそれぞれと一致するかで比べる。
func (definition serviceDefinitionFile) isOutdated() (bool, error) {
	snapshot, err := definition.readSnapshot()
	// readSnapshot は、sshc の印がある定義を serviceInactive と返す。
	if err != nil || snapshot.state != serviceInactive {
		return false, err
	}
	template, err := definition.render(serviceExecutablePlaceholder)
	if err != nil {
		return false, err
	}
	before, after, found := strings.Cut(template, serviceExecutablePlaceholder)
	if !found {
		return false, fmt.Errorf("%s does not name the executable in one place", definition.name)
	}
	contents := string(snapshot.contents)
	current := len(contents) > len(before)+len(after) &&
		strings.HasPrefix(contents, before) && strings.HasSuffix(contents, after)
	return !current, nil
}

// outdatedDefinitionError は、定義が以前の版の形のままなら errOutdatedServiceDefinition を返す。
func outdatedDefinitionError(definition serviceDefinitionFile) error {
	outdated, err := definition.isOutdated()
	if err != nil {
		return err
	}
	if outdated {
		return errOutdatedServiceDefinition
	}
	return nil
}

// ensureStillMatches は、engine の起動を待つ間に定義が書き換えられていれば失敗する。
// activity は、その間に service が何をしていたか（starting、restarting）である。
func (definition serviceDefinitionFile) ensureStillMatches(executable, activity string) error {
	matches, err := definition.matches(executable)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("%s changed while the service was %s", definition.name, activity)
	}
	return nil
}
