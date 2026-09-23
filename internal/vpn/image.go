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
)

// container には、VPNコンテナのイメージを作るものだけを置く。sshcのバイナリも
// 利用者の設定も入れない。イメージは依存物だけを含む。
//
//go:embed container/Dockerfile container/agent.sh container/vpnc-script
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
// 無ければ作る。
func (manager *Manager) ensureImage(ctx context.Context) (string, error) {
	tag, err := imageTag()
	if err != nil {
		return "", err
	}
	if _, err := manager.docker.output(ctx, "image", "inspect", tag); err == nil {
		return tag, nil
	}
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
	if _, err := manager.docker.output(ctx, "build", "--tag", tag, directory); err != nil {
		return "", fmt.Errorf("%w: %w", ErrImageBuild, err)
	}
	return tag, nil
}
