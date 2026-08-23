package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/validate"
)

// ConfigFile は Include グラフのファイルひとつを、表示用に要約したもの。
type ConfigFile struct {
	Path     string
	Editable bool
	Missing  bool
	Loads    int
	Includes int
}

// ConfigReport は構文と Include のチェック。プロセスは何も起動しない。
type ConfigReport struct {
	Root        string
	Files       []ConfigFile
	Diagnostics []config.Diagnostic
}

// Inspection は、実効設定の画面が必要とするすべて。
//
// **ssh を回さないので、実行の可否も確認も結果もここには無い。** 値を決めるのは
// effective.Resolve であり、この検査が運ぶのは出所と、単純に説明できない理由と、
// ジャンプ経路である。
type Inspection struct {
	Alias             string
	Report            effective.Report
	Projection        effective.Projection
	Route             []effective.Stage
	RouteComplexities []effective.Complexity
}

// ConnectionSnapshot は、1 回だけ読み取った設定グラフから導出された接続計画と、
// OpenSSH に渡せる単一の不変設定。
type ConnectionSnapshot struct {
	Hostname string
	Port     string
	User     string
	Report   effective.Report
	Config   []byte
}

// Service は、設定エンジンとこのパッケージのチェックを組み合わせる。リクエストの
// たびに設定を読み直す。ファイルこそが真実の源であり、二つのリクエストのあいだに
// 変わりうるからである。
type Service struct {
	Workspace      *storage.Workspace
	Resolver       config.Resolver
	Reachability   Reachability
	Authentication Authentication
	// Facts は、解決に要るこのプロセスの事実である。
	//
	// **注入する。** ここで user.Current() を読めば、テストが本物のアカウント名に
	// 届く——`Match localuser` を含む設定の答えが、走らせた人によって変わる。
	Facts effective.LocalFacts
}

// NewService は本番用の依存を配線する。
//
// probe は、接続ひとつ分を組み立てて認証だけを試す。**外部プログラムは
// 起こさない。** nil なら認証テストは「手段が無い」と答える——できないことを
// できるふりで隠さない。
func NewService(
	workspace *storage.Workspace,
	probe func(ctx context.Context, alias string) (sshclient.Probe, error),
	facts effective.LocalFacts,
) *Service {
	return &Service{
		Workspace:      workspace,
		Resolver:       storage.NewResolver(workspace),
		Reachability:   Reachability{Dialer: &net.Dialer{}},
		Authentication: Authentication{Dial: probe},
		Facts:          facts,
	}
}

// ConfigPath は、このサービスが評価するユーザー設定。
func (s *Service) ConfigPath() string { return filepath.Join(s.Workspace.Root(), "config") }

// Home はユーザーのホームディレクトリ。取り込んだ出力がこのプロセスから出ていく
// 前に、それを浄化するために使う。
func (s *Service) Home() string { return s.Workspace.Home() }

func (s *Service) graph() (*config.Graph, error) { return s.Resolver.Resolve(s.ConfigPath()) }

// Safety は、現在の設定に実行を伴うディレクティブがないか走査する。
func (s *Service) Safety() (effective.Report, error) {
	graph, err := s.graph()
	if err != nil {
		return effective.Report{}, err
	}
	return effective.Scan(graph), nil
}

// ConnectionSnapshot は宛先、ユーザー、安全性、実行用設定を同じ Graph から
// 導出する。呼び出し中に設定が変わっても、互いに異なる世代を混ぜない。
func (s *Service) ConnectionSnapshot(alias string) (ConnectionSnapshot, error) {
	if err := validate.Alias(alias); err != nil {
		return ConnectionSnapshot{}, err
	}
	graph, err := s.graph()
	if err != nil {
		return ConnectionSnapshot{}, err
	}
	flattened, err := config.Snapshot(graph)
	if err != nil {
		return ConnectionSnapshot{}, err
	}
	// **Project ではなく Resolve から取る。** ここで返す値は、公開鍵を
	// authorized_keys へ入れてよいかを人に確かめてもらう画面に出る。実際に繋ぐのは
	// remotekey.Service.Resolve で、そちらは既に Resolve を通っていた。**つまり
	// 見せていた宛先と、繋ぐ宛先が違いえた** ——Match の下に HostName を書いている人は、
	// 「bastion.internal に入れます」と確認したうえで別の機械に入れることになる。
	resolution := effective.Resolve(graph, alias, s.Facts)
	if len(resolution.Refusals) > 0 {
		// **推測で埋めない。** どこへ繋がるか分からない設定について、alias を
		// そのままホスト名として見せれば、人は確かめようのないものを確かめる。
		return ConnectionSnapshot{}, fmt.Errorf("%w: %s", ErrUnresolvedDestination, resolution.Refusals[0].Code)
	}
	snapshot := ConnectionSnapshot{
		Hostname: alias,
		Port:     "22",
		Report:   effective.Scan(graph),
		Config:   flattened,
	}
	if found := resolution.Values.First("hostname"); found != "" {
		snapshot.Hostname = found
	}
	if found := resolution.Values.First("port"); found != "" {
		snapshot.Port = found
	}
	if found := resolution.Values.First("user"); found != "" {
		snapshot.User = found
	}
	return snapshot, nil
}

// ConfigCheck は、Include グラフとその診断を報告する。
func (s *Service) ConfigCheck() (ConfigReport, error) {
	graph, err := s.graph()
	if err != nil {
		return ConfigReport{}, err
	}
	report := ConfigReport{Root: graph.Root, Diagnostics: graph.Diagnostics}
	for _, path := range graph.Order {
		node := graph.Nodes[path]
		if node == nil {
			continue
		}
		report.Files = append(report.Files, ConfigFile{
			Path:     node.Path,
			Editable: node.Editable,
			Missing:  node.Missing,
			Loads:    node.Loads,
			Includes: len(node.Includes),
		})
	}
	return report, nil
}

// Inspect は alias ひとつを説明し、許されている場合はそれを評価する。
//
// 拒否された評価も、失敗した ssh も、どちらもデータとして返る。画面には、エンジン
// 自身の射影と、先に確認しなければならないコマンドそのものが引き続き表示
// される。
func (s *Service) Inspect(alias string) (Inspection, error) {
	if err := validate.Alias(alias); err != nil {
		return Inspection{}, err
	}
	graph, err := s.graph()
	if err != nil {
		return Inspection{}, err
	}

	inspection := Inspection{Alias: alias, Report: effective.Scan(graph)}
	inspection.Projection = effective.Project(graph, alias)
	inspection.Route, inspection.RouteComplexities = effective.ExpandRoute(graph, alias, s.Facts)
	return inspection, nil
}

// Destination は、エンジンが alias に対して射影するホスト名とポートを返す。
//
// ssh を実行しないので、実行を伴うディレクティブによって評価が阻まれている
// あいだも到達性のチェックは機能する。
func (s *Service) Destination(alias string) (string, string, error) {
	if err := validate.Alias(alias); err != nil {
		return "", "", err
	}
	graph, err := s.graph()
	if err != nil {
		return "", "", err
	}
	// **Project ではなく Resolve から取る。** ここが答えるのは「どの行が書いたか」
	// ではなく、**実際にダイヤルする宛先**である。Project は Match ブロックを
	// 一切適用しないので、Match の下に HostName や Port を書いている人に対して、
	// 到達性の検査は違う相手を叩いていた——そして「繋がりません」と答えていた。
	resolution := effective.Resolve(graph, alias, s.Facts)
	if len(resolution.Refusals) > 0 {
		// **推測で埋めない。** 解決を諦めた設定について、alias:22 を叩いた
		// 結果を「その接続先への到達性」として見せるのは嘘である。
		return "", "", fmt.Errorf("%w: %s", ErrUnresolvedDestination, resolution.Refusals[0].Code)
	}
	hostname := alias
	if found := resolution.Values.First("hostname"); found != "" {
		hostname = found
	}
	port := "22"
	if found := resolution.Values.First("port"); found != "" {
		port = found
	}
	return hostname, port, nil
}

// ErrUnresolvedDestination は、この設定について宛先を言えないことを報告する。
//
// **黙って alias:22 を返さない。** Match exec を含む設定では、どこへ繋がるかが
// まさに分からない。分からないものを叩いた結果を到達性として見せない。
var ErrUnresolvedDestination = errors.New("this configuration does not resolve to one destination")

// Reach は接続先へ直接ダイヤルし、ProxyJump を無視する。
func (s *Service) Reach(ctx context.Context, alias string) (ReachabilityResult, error) {
	hostname, port, err := s.Destination(alias)
	if err != nil {
		return ReachabilityResult{}, err
	}
	if err := validate.Hostname(hostname); err != nil {
		return ReachabilityResult{}, err
	}
	return s.Reachability.Check(ctx, hostname, port), nil
}

// Authenticate は、alias に対する認証テストを実行する。
//
// 取り込んだ stderr はユーザーに表示されるので、先にホームディレクトリを "~" に
// 書き換える。冗長な OpenSSH の出力は、読んだファイルをすべて絶対パスで名指しする
// ため、そうしないとアカウント名がレスポンスの本文へ運ばれてしまう。
func (s *Service) Authenticate(ctx context.Context, alias string, acknowledged bool) (AuthenticationResult, error) {
	report, err := s.Safety()
	if err != nil {
		return AuthenticationResult{}, err
	}
	result, err := s.Authentication.Test(ctx, report, alias, acknowledged)
	if err != nil {
		return AuthenticationResult{}, err
	}
	result.Detail = platform.SanitiseHomePaths(result.Detail, s.Workspace.Home())
	return result, nil
}
