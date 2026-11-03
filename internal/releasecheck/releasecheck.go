// Package releasecheck は、プロジェクトの GitHub リリースを調べ、より新しいものが
// あるかを伝える。
//
// リリース情報を返し、更新の実行は既存のinstall managerへ委ねる。
package releasecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// LatestAPI は、プロジェクトの最新リリースを返す GitHub のエンドポイントである。
const LatestAPI = "https://api.github.com/repos/aida0710/sshc/releases/latest"

// LatestPage は、最新リリースのページへ転送する GitHub のページである。API は認証なしだと
// 1 つの IP アドレスにつき 1 時間 60 回までで、職場などで共有すると使い切る。このページはその回数に数えない。
const LatestPage = "https://github.com/aida0710/sshc/releases/latest"

// maxLatestResponseBytes bounds the single-release JSON response before decoding.
const maxLatestResponseBytes = 1 << 20

// ErrNoRelease は、プロジェクトがまだ何も公開していないことを報告する。
var ErrNoRelease = errors.New("no release has been published")

// Release は、プロジェクトが公開したもの。
type Release struct {
	Version     string
	PageURL     string
	PublishedAt time.Time
	Body        string
}

// Checker は、最新リリースが何かを GitHub に尋ねる。
type Checker struct {
	// API はリリースのエンドポイント。テストが GitHub に到達しないよう注入する。
	API string
	// Page は、API が断ったときに最新の版を尋ねるページである。空なら尋ねない。
	Page string
	HTTP *http.Client
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
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
		return c.latestFromPage(ctx, fmt.Errorf("the releases API answered %d", response.StatusCode))
	}

	var decoded githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, maxLatestResponseBytes)).Decode(&decoded); err != nil {
		return Release{}, err
	}
	if _, stable := StableTag(decoded.TagName); decoded.Draft || decoded.Prerelease || !stable {
		return Release{}, ErrNoRelease
	}
	return decoded.release(), nil
}

// latestFromPage は、API が refusal で断った代わりに、Page の転送先から最新の版を読む。
// 分かるのは版とページだけで、公開日時と本文は分からない。
func (c Checker) latestFromPage(ctx context.Context, refusal error) (Release, error) {
	if c.Page == "" {
		return Release{}, refusal
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Page, nil)
	if err != nil {
		return Release{}, fmt.Errorf("%w; %w", refusal, err)
	}
	client := *c.client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("%w; %w", refusal, err)
	}
	defer func() { _ = response.Body.Close() }()
	location, err := response.Location()
	if err != nil {
		return Release{}, fmt.Errorf("%w; the latest release page answered %d", refusal, response.StatusCode)
	}
	// リリースが 1 つも無いと、タグの無いリリースの一覧へ転送される。
	tag := path.Base(location.Path)
	if stable, ok := StableTag(tag); !ok || stable != tag || path.Base(path.Dir(location.Path)) != "tag" {
		return Release{}, ErrNoRelease
	}
	return Release{Version: tag, PageURL: location.String()}, nil
}

// Newer は、candidate が current より後のバージョンかを報告する。
//
// candidate はネットワークから来る値なので、SemVerでない値を更新として扱わない。
// リリースでないローカルビルド（"dev"など）には、正規のリリースがあることだけを
// 伝える。SemVerのpre-release順序も比較し、安定バージョンから古いpre-releaseへ戻さない。
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

// StableTag は、GitHub Releaseとinstallerへ渡してよい正規の安定バージョンのtagを返す。
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
