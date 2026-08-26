// Package selfupdate は、プロジェクトの GitHub リリースを調べ、より新しいものが
// あるかを伝える。
//
// 新しいバージョンとリリース情報の URL のみを返す。このpackage自身は更新の
// downloadやbinary置換を行わず、呼び出し側が既存のinstall managerへ委ねる。
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"
)

// ErrNoRelease は、プロジェクトがまだ何も公開していないことを報告する。
var ErrNoRelease = errors.New("no release has been published")

// Release は、プロジェクトが公開したもの。
type Release struct {
	Version string
	// PageURL は、何が変わったかをユーザーが読み、どうするかを決める場所。これが提供する
	// もののすべてである。
	PageURL string
}

// Checker は、最新リリースが何かを GitHub に尋ねる。
type Checker struct {
	// API はリリースのエンドポイント。テストが GitHub に到達しないよう注入する。
	API  string
	HTTP *http.Client
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

func (c Checker) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Latest は、公開されている最も新しいリリースを返す。
func (c Checker) Latest(ctx context.Context) (Release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := c.client().Do(request)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return Release{}, ErrNoRelease
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("the releases API answered %d", response.StatusCode)
	}

	var decoded githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decoded); err != nil {
		return Release{}, err
	}
	if decoded.Draft || decoded.TagName == "" {
		return Release{}, ErrNoRelease
	}
	return Release{Version: decoded.TagName, PageURL: decoded.HTMLURL}, nil
}

// Newer は、candidate が current より後のバージョンかを報告する。
//
// candidate はネットワークから来る値なので、SemVerでない値を更新として扱わない。
// リリースでないローカルビルド（"dev"など）には、正規のリリースがあることだけを
// 伝える。SemVerのpre-release順序も比較し、安定版から古いpre-releaseへ戻さない。
func Newer(current, candidate string) bool {
	if current == candidate {
		return false
	}
	candidateVersion, candidateOK := semanticVersion(candidate)
	if !candidateOK {
		return false
	}
	currentVersion, currentOK := semanticVersion(current)
	if !currentOK {
		return true
	}
	return semver.Compare(candidateVersion, currentVersion) > 0
}

// StableTag は、GitHub Releaseとinstallerへ渡してよい正規の安定版tagを返す。
// update処理はこの検査を通らないtagからURLや環境変数を組み立てない。
func StableTag(value string) (string, bool) {
	parsed, ok := semanticVersion(value)
	if !ok || semver.Prerelease(parsed) != "" || semver.Build(parsed) != "" {
		return "", false
	}
	return parsed, true
}

func semanticVersion(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value, semver.IsValid(value)
}
