package diagnostics

import (
	"context"
	"net"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
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
}

// NewService は本番用の依存を配線する。
//
// probe は、接続ひとつ分を組み立てて認証だけを試す。**外部プログラムは
// 起こさない。** nil なら認証テストは「手段が無い」と答える——できないことを
// できるふりで隠さない。
func NewService(
	workspace *storage.Workspace,
	probe func(ctx context.Context, alias string) (sshclient.Probe, error),
) *Service {
	return &Service{
		Workspace:      workspace,
		Resolver:       storage.NewResolver(workspace),
		Reachability:   Reachability{Dialer: &net.Dialer{}},
		Authentication: Authentication{Dial: probe},
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
	if err := platform.ValidateAlias(alias); err != nil {
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
	projection := effective.Project(graph, alias)
	snapshot := ConnectionSnapshot{
		Hostname: alias,
		Port:     "22",
		Report:   effective.Scan(graph),
		Config:   flattened,
	}
	if source, ok := projection.Value("hostname"); ok {
		snapshot.Hostname = source.Value
	}
	if source, ok := projection.Value("port"); ok {
		snapshot.Port = source.Value
	}
	if source, ok := projection.Value("user"); ok {
		snapshot.User = source.Value
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
	if err := platform.ValidateAlias(alias); err != nil {
		return Inspection{}, err
	}
	graph, err := s.graph()
	if err != nil {
		return Inspection{}, err
	}

	inspection := Inspection{Alias: alias, Report: effective.Scan(graph)}
	inspection.Projection = effective.Project(graph, alias)
	inspection.Route, inspection.RouteComplexities = effective.ExpandRoute(graph, alias)
	return inspection, nil
}

// Destination は、エンジンが alias に対して射影するホスト名とポートを返す。
//
// ssh を実行しないので、実行を伴うディレクティブによって評価が阻まれている
// あいだも到達性のチェックは機能する。
func (s *Service) Destination(alias string) (string, string, error) {
	if err := platform.ValidateAlias(alias); err != nil {
		return "", "", err
	}
	graph, err := s.graph()
	if err != nil {
		return "", "", err
	}
	projection := effective.Project(graph, alias)
	hostname := alias
	if source, ok := projection.Value("hostname"); ok {
		hostname = source.Value
	}
	port := "22"
	if source, ok := projection.Value("port"); ok {
		port = source.Value
	}
	return hostname, port, nil
}

// ProjectedValue は、alias に対するキーワードひとつについてエンジン自身の読みを返す。
//
// Destination と同様にプロセスを起動しないので、実行を伴うディレクティブで評価が
// 阻まれているあいだも、呼び出し側は接続先を記述できる。この値は OpenSSH の答え
// ではなくエンジンの射影であり、それを表示する呼び出し側は、その旨を述べなければ
// ならない。
func (s *Service) ProjectedValue(alias, keyword string) (string, bool) {
	if err := platform.ValidateAlias(alias); err != nil {
		return "", false
	}
	graph, err := s.graph()
	if err != nil {
		return "", false
	}
	source, ok := effective.Project(graph, alias).Value(keyword)
	if !ok {
		return "", false
	}
	return source.Value, true
}

// Reach は接続先へ直接ダイヤルし、ProxyJump を無視する。
func (s *Service) Reach(ctx context.Context, alias string) (ReachabilityResult, error) {
	hostname, port, err := s.Destination(alias)
	if err != nil {
		return ReachabilityResult{}, err
	}
	if err := platform.ValidateHostname(hostname); err != nil {
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
