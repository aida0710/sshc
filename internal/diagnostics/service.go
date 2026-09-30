package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"sshc/internal/config"
	"sshc/internal/configresolver"
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

// Inspection は、実効設定の出所、評価を拒否した理由、ジャンプ経路を保持する。
// SSH の実行結果は含まない。
type Inspection struct {
	Alias             string
	Report            effective.Report
	Projection        effective.Projection
	Route             []effective.Stage
	RouteComplexities []effective.Complexity
}

// ConnectionSnapshot は、1 回だけ読み取った設定グラフから導出された接続計画と、
// 確認の証拠にする設定のバイト列（config.Snapshot）。
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
	// Facts は Match localuser などの解決に使うローカル環境情報。
	Facts effective.LocalFacts
	// VPNBinding は、alias が通る VPN プロファイルの名前を返す。空なら VPN を通らない。
	// 紐付けは ~/.ssh/config ではなく sshc の設定にあるので、設定グラフからは読めない。
	// nil なら、どの接続も VPN を通らないものとして扱う。
	VPNBinding func(alias string) (string, error)
}

// NewService は本番用の依存を配線する。
//
// probe はプロセス内で認証だけを試す。nil の場合、認証テストは利用不可となる。
func NewService(
	workspace *storage.Workspace,
	probe func(ctx context.Context, alias string) (sshclient.Probe, error),
	facts effective.LocalFacts,
) *Service {
	return &Service{
		Workspace:      workspace,
		Resolver:       configresolver.ForWorkspace(workspace),
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
	// 公開鍵登録の確認画面と実際の接続先を一致させるため、Project ではなく
	// Resolve で接続先を決定する。
	resolution := effective.Resolve(graph, alias, s.Facts)
	if len(resolution.Refusals) > 0 {
		// 接続先を解決できない場合は alias をホスト名として代用しない。
		return ConnectionSnapshot{}, fmt.Errorf("%w: %s", ErrUnresolvedDestination, resolution.Refusals[0].Code)
	}
	// HostName、Port、User の既定値は Resolve が埋めている。ここで二度目の既定を持たない。
	return ConnectionSnapshot{
		Hostname: resolution.Values.First("hostname"),
		Port:     resolution.Values.First("port"),
		User:     resolution.Values.First("user"),
		Report:   effective.ScanForAlias(graph, alias),
		Config:   flattened,
	}, nil
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

// Inspect は alias ひとつを、設定を読むだけで説明する。
//
// ssh も設定に書かれたコマンドも実行しないので、呼び出し側の確認は要らない。返すのは、
// エンジン自身の射影、ProxyJump で経由するホストの並び、設定から届く実行を伴う
// ディレクティブの一覧（Report）である。画面はこの一覧を見せてから、認証テストの前に
// 確認を求める。
func (s *Service) Inspect(alias string) (Inspection, error) {
	if err := validate.Alias(alias); err != nil {
		return Inspection{}, err
	}
	graph, err := s.graph()
	if err != nil {
		return Inspection{}, err
	}

	inspection := Inspection{Alias: alias, Report: effective.ScanForAlias(graph, alias)}
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
	// 実際の接続先を返すため、Match ブロックを適用しない Project ではなく
	// Resolve を使用する。
	resolution := effective.Resolve(graph, alias, s.Facts)
	if len(resolution.Refusals) > 0 {
		// 解決できない場合は alias:22 を接続先として代用しない。
		return "", "", fmt.Errorf("%w: %s", ErrUnresolvedDestination, resolution.Refusals[0].Code)
	}
	// 既定の HostName（alias）と Port は Resolve が埋めている。
	return resolution.Values.First("hostname"), resolution.Values.First("port"), nil
}

// ErrUnresolvedDestination は、設定から単一の接続先を解決できないことを表す。
var ErrUnresolvedDestination = errors.New("this configuration does not resolve to one destination")

// ErrUnsafeDestination は、Reach が設定から、ダイヤルしてよい接続先を1つに決められないことを
// 表す。接続先を解決できない場合と、ホスト名が安全でない場合である。
var ErrUnsafeDestination = errors.New("the destination is not safe to dial")

// Reach は接続先へ直接ダイヤルし、ProxyJump を無視する。
//
// VPN プロファイルを付けた接続ではダイヤルしない。VPN の中でしか引けない名前を
// このマシンの DNS へ問い合わせ、VPN の中にしか無いアドレスへこのマシンの回線で
// 繋ぐことになり、結果が実際の接続経路と関係しないからである。
func (s *Service) Reach(ctx context.Context, alias string) (ReachabilityResult, error) {
	hostname, port, err := s.Destination(alias)
	if errors.Is(err, ErrUnresolvedDestination) {
		return ReachabilityResult{}, fmt.Errorf("%w: %w", ErrUnsafeDestination, err)
	}
	if err != nil {
		// 設定を読めないのは、接続先が安全でないことを意味しない。
		return ReachabilityResult{}, err
	}
	if err := validate.Hostname(hostname); err != nil {
		return ReachabilityResult{}, fmt.Errorf("%w: %w", ErrUnsafeDestination, err)
	}
	routedThroughVPN, err := s.routedThroughVPN(alias)
	if err != nil {
		return ReachabilityResult{}, err
	}
	if routedThroughVPN {
		return ReachabilityResult{
			Address: net.JoinHostPort(hostname, port),
			Outcome: ReachabilityNotChecked,
			Notice:  VPNRouteNotice,
		}, nil
	}
	return s.Reachability.Check(ctx, hostname, port), nil
}

// routedThroughVPN は、alias に VPN プロファイルが付いているかを返す。
func (s *Service) routedThroughVPN(alias string) (bool, error) {
	if s.VPNBinding == nil {
		return false, nil
	}
	profile, err := s.VPNBinding(alias)
	if err != nil {
		return false, err
	}
	return profile != "", nil
}

// Authenticate は、alias に対する認証テストを実行する。
//
// 認証はプロセス内の probe が一度だけ試し、外部のプログラムは起動しない。
// 結果の Detail は利用者に表示するので、先にホームディレクトリを "~" に書き換える。
// 鍵を読めなかった理由やサーバーの案内には IdentityFile などの絶対パスが入りうるので、
// そうしないとアカウント名がレスポンスの本文へ運ばれてしまう。
func (s *Service) Authenticate(ctx context.Context, alias string, acknowledged bool) (AuthenticationResult, error) {
	if err := validate.Alias(alias); err != nil {
		return AuthenticationResult{}, err
	}
	graph, err := s.graph()
	if err != nil {
		return AuthenticationResult{}, err
	}
	report := effective.ScanForAlias(graph, alias)
	result, err := s.Authentication.Test(ctx, report, alias, acknowledged)
	if err != nil {
		return AuthenticationResult{}, err
	}
	result.Detail = platform.SanitiseHomePaths(result.Detail, s.Workspace.Home())
	return result, nil
}
