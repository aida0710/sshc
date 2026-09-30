package vpn

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sshc/internal/connectionlog"
)

// container には、VPNコンテナのイメージを作るものだけを置く。sshcのバイナリも
// 利用者の設定も入れない。イメージは依存物だけを含む。
//
//go:embed container
var container embed.FS

// defaultImageName は、sshc が経路に使うイメージの名前である。タグは中身から決まる。
// VPN の結合テストは別の名前を使う（Manager.imageName）。
const defaultImageName = "sshc-vpn"

// image は、この Manager が使うイメージ（名前:タグ）を返す。
func (manager *Manager) image() (string, error) {
	tag, err := imageTag()
	if err != nil {
		return "", err
	}
	return manager.imageName + ":" + tag, nil
}

// imageTag は、埋め込んだ内容から決まるタグを返す。
//
// 内容が変わればタグも変わるので、古いイメージが残っているマシンでも、新しい
// sshcは自分が知っている内容のイメージだけを使う。
func imageTag() (string, error) {
	digest := sha256.New()
	names, err := containerFileNames()
	if err != nil {
		return "", err
	}
	for _, name := range names {
		contents, err := container.ReadFile(name)
		if err != nil {
			return "", err
		}
		digest.Write([]byte(name))
		digest.Write(contents)
	}
	return hex.EncodeToString(digest.Sum(nil))[:imageTagLength], nil
}

// imageTagLength は、タグにする SHA-256 の16進の桁数である。別の中身と取り違えない
// 長さのうち、docker image ls で読める短さにする。
const imageTagLength = 12

func containerFileNames() ([]string, error) {
	entries, err := container.ReadDir("container")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, filepath.ToSlash(filepath.Join("container", entry.Name())))
	}
	sort.Strings(names)
	return names, nil
}

// ensureImage は、このsshcが知っている内容のイメージがあることを確かめ、
// 無ければ作る。作るときだけ、report に PhaseImage を知らせる。
func (manager *Manager) ensureImage(ctx context.Context, report func(StartPhase)) (string, error) {
	tag, err := manager.image()
	if err != nil {
		return "", err
	}
	if _, present, err := manager.docker.probe(ctx, "コンテナイメージ", "image", "inspect", tag); err == nil && present {
		connectionlog.Say(ctx, connectionlog.Detailed, "コンテナイメージ「%s」は作成済みです。", tag)
		return tag, nil
	}
	report(PhaseImage)
	connectionlog.Say(ctx, connectionlog.Notice, "%s", PhaseImage.Notice())
	connectionlog.Say(ctx, connectionlog.Detailed, "作成するコンテナイメージ：%s", tag)
	directory, err := os.MkdirTemp("", "sshc-vpn-image-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	names, err := containerFileNames()
	if err != nil {
		return "", err
	}
	for _, name := range names {
		contents, err := container.ReadFile(name)
		if err != nil {
			return "", err
		}
		path := filepath.Join(directory, filepath.Base(name))
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			return "", err
		}
	}
	started := time.Now()
	connectionlog.Progress(ctx, connectionlog.Full, "docker buildの出力：")
	eachLine := func(line string) { connectionlog.Progress(ctx, connectionlog.Full, "  %s", line) }
	if err := manager.docker.build(ctx, tag, directory, eachLine); err != nil {
		if stopCauseOf(ctx) != nil {
			// 停止で打ち切った作成は失敗ではない。出力も失敗として写さない。
			return "", err
		}
		connectionlog.Say(ctx, connectionlog.Brief, "コンテナイメージの作成に失敗しました（%s）。",
			time.Since(started).Round(time.Second))
		connectionlog.Say(ctx, connectionlog.Detailed, "docker buildの出力（最後の%d行まで）：", maxShownOutputLines)
		sayOutput(ctx, connectionlog.Detailed, err.Error())
		// 出力は上で写した。エラーの文には、失敗の要点の1行だけを残す。
		return "", fmt.Errorf("%w: %s", ErrImageBuild, buildFailureSummary(err.Error()))
	}
	connectionlog.Say(ctx, connectionlog.Detailed, "コンテナイメージを作成しました（%s）。", time.Since(started).Round(time.Second))
	manager.removeOtherImages(ctx, tag)
	return tag, nil
}

// buildFailureSummary は、docker build の出力から失敗の要点の1行を選ぶ。BuildKit は
// 最後に「ERROR: failed to build: …」の行を書くので、ERROR で始まる最後の行を選ぶ。
// 無ければ、空でない最後の行を選ぶ。
func buildFailureSummary(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); strings.HasPrefix(line, "ERROR") {
			return line
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// removeOtherImages は、同じ名前で中身の違うイメージ（current 以外のタグ）を消す。
//
// タグは中身から決まるので、sshc を更新するたびに新しいイメージが1つ増える。消さないと、
// 使わなくなったものが数百 MB ずつ残り続ける。どのバージョンの sshc が作ったかは
// 見ないので、同じ Docker を使う、中身の違う sshc（新しいバージョンも含む）のイメージも
// 消える。消された側は、次に経路を起動するときに作り直す。タグが2つ以上あるイメージは、
// そのタグだけが外れる。動いているコンテナが使っているイメージは docker が消させない
// ので、その失敗は無視する。
//
// 名前は Manager ごとに決まる。結合テストは別の名前を使うので、インストールした sshc の
// イメージはここで消えない。
func (manager *Manager) removeOtherImages(ctx context.Context, current string) {
	output, err := manager.docker.output(ctx, "image", "ls", manager.imageName, "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return
	}
	for _, image := range strings.Fields(output) {
		if image != current {
			_, _ = manager.docker.output(ctx, "image", "rm", image)
		}
	}
}
