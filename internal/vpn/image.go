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

// imageName は、このイメージの名前である。タグは中身から決まる。
const imageName = "sshc-vpn"

// imageTag は、埋め込んだ内容から決まるタグを返す。
//
// 内容が変わればタグも変わるので、古いイメージが残っている機械でも、新しい
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
	return imageName + ":" + hex.EncodeToString(digest.Sum(nil))[:12], nil
}

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
	tag, err := imageTag()
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

// removeOtherImages は、前のバージョンの sshc が作ったイメージを消す。
//
// タグは中身から決まるので、sshc を更新するたびに新しいイメージが1つ増える。
// 古いものは誰も使わないまま、数百 MB ずつ残り続ける。まだ動いている
// コンテナが使っているイメージは docker が消させないので、その失敗は無視する。
func (manager *Manager) removeOtherImages(ctx context.Context, current string) {
	output, err := manager.docker.output(ctx, "image", "ls", imageName, "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return
	}
	for _, image := range strings.Fields(output) {
		if image != current {
			_, _ = manager.docker.output(ctx, "image", "rm", image)
		}
	}
}
