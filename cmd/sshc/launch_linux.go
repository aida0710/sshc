//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// desktopDescriptorName は、外殻が自分の居場所を書き残す先である。
//
// AppImage は動く。利用者はダウンロードしたものを後から片づけるし、置き場所を
// 決めるのは利用者であってこのアプリケーションではない。だから場所を推測せず、
// 上がるたびに外殻自身が書いた一箇所だけを読む。
const desktopDescriptorName = "desktop.json"

// desktopDescriptor は、外殻が書き残す内容である。**資格情報は入らない。**
type desktopDescriptor struct {
	Executable string `json:"executable"`
}

// linuxDesktop は、記録された実体を直接起こす。
//
// **shell を通さないし、PATH も引かない。** 記録されているのは絶対パスひとつで、
// それを exec する。名前で引けば、誰の PATH に何が置かれているかで起こすものが
// 変わる——それは利用者が意図していない実行である。
type linuxDesktop struct {
	stateDir string
	lookup   func(string) (string, bool)
}

func newDesktopLauncher() desktopLauncher {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return linuxDesktop{stateDir: linuxDescriptorDir(home), lookup: os.LookupEnv}
}

func linuxDescriptorDir(home string) string { return filepath.Join(home, ".ssh", "sshc") }

// Available は、画面があり、記録された実体がいまも起こせるかを答える。
//
// 画面が無いことは壊れていることではないので error にしない——その計算機では
// `sshc headless` が正しい答えであり、直すものは何も無い。記録が古いことは
// 壊れていることなので、直し方と一緒に返す。
func (launcher linuxDesktop) Available() (bool, error) {
	if !hasDisplay(launcher.lookup) {
		return false, nil
	}
	path, err := readDesktopDescriptor(launcher.stateDir)
	if err != nil {
		return false, err
	}
	if err := validateDesktopExecutable(path); err != nil {
		return false, err
	}
	return true, nil
}

func (launcher linuxDesktop) Launch(ctx context.Context) error {
	path, err := readDesktopDescriptor(launcher.stateDir)
	if err != nil {
		return err
	}
	if err := validateDesktopExecutable(path); err != nil {
		return err
	}
	// 二つ目の実体は、既存の実体へ知らせて自分は終わる。待たないのは、外殻が
	// 上がりきるより先に端末を返してよいからである。
	if err := exec.CommandContext(ctx, path).Start(); err != nil {
		return fmt.Errorf("could not start %s; open the sshc AppImage once at its current location", path)
	}
	return nil
}

func hasDisplay(lookup func(string) (string, bool)) bool {
	for _, name := range []string{"DISPLAY", "WAYLAND_DISPLAY"} {
		if value, present := lookup(name); present && value != "" {
			return true
		}
	}
	return false
}

func readDesktopDescriptor(stateDir string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(stateDir, desktopDescriptorName))
	if err != nil {
		return "", errors.New("no sshc desktop application has been started here; open the sshc AppImage once")
	}
	var descriptor desktopDescriptor
	if err := json.Unmarshal(contents, &descriptor); err != nil {
		return "", errors.New("the recorded sshc desktop location is unreadable; open the sshc AppImage once")
	}
	return descriptor.Executable, nil
}

// validateDesktopExecutable は、記録された実体をそのまま起こしてよいかを見る。
//
// **絶対パスの、通常ファイルで、実行できるもの**だけを通す。相対パスは PATH と
// 作業ディレクトリに意味を与えてしまい、ディレクトリやリンク先の消えたものは
// 起こせない。古い AppImage を探し回って一番それらしいものを起こす、ということ
// もしない——どれが利用者のものかを、このアプリケーションは知らない。
func validateDesktopExecutable(path string) error {
	moved := fmt.Errorf("the recorded sshc desktop application is no longer at %s; open the AppImage once at its new location", path)
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("the recorded sshc desktop location is not an absolute path; open the sshc AppImage once")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return moved
	}
	if info.Mode().Perm()&0o111 == 0 {
		return moved
	}
	return nil
}

// launchBackground は、接続経路が engine を必要としたときに外殻を起こす。
// 起こせたかどうかしか返らない——接続そのものは、起きなくても続く。
func launchBackground(ctx context.Context) bool {
	launcher := newDesktopLauncher()
	if available, err := launcher.Available(); err != nil || !available {
		return false
	}
	return launcher.Launch(ctx) == nil
}
