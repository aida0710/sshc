// Package terminalopen は、ターミナルのセッションを組み立てて開く。接続名への SSH の
// セッションと、このマシンのシェルの2種類がある。
//
// HTTP の受け口（httpserver）は、要求の形を確かめてここを呼び、結果を答えに変えるだけにする。
// 前のエンジンが開いていたセッションの開き直し（Reopen）も同じ組み立てを通る。
package terminalopen

import (
	"context"
	"errors"
	"path/filepath"

	"sshc/internal/platform"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
	"sshc/internal/terminalworkspace"
	"sshc/internal/validate"
)

// ErrMissingAlias は、SSH のセッションを開くのに接続名が無いことを表す。
var ErrMissingAlias = errors.New("an ssh session needs an alias")

// Connector は、alias ひとつ分の対話セッションを開く。
//
// 外部の ssh は起こさない。組み立てるのは合成の根（internal/app）であり、
// 鍵も vault も known_hosts もそこで一度だけ配線される。二箇所で組み立てると、
// 片方だけが vault を見る日が来る。
type Connector func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error)

// StartupSnippet は、接続のたびに起動スニペットについて決めたことである。
//
// Command があれば、認証とリモートシェルの起動が済んでから送る。Notice があれば、
// 同じ時点でターミナルの接続ログへ 1 行書く。どちらも空なら何もしない。
type StartupSnippet struct {
	// Command は、変数（シークレットを含む）を展開したコマンドである。
	Command string
	// Notice は、送らなかったことを利用者に知らせる 1 行である。割り当て直すなど、
	// 利用者の操作で直せる理由のときだけ入れる。
	Notice string
}

// Request は、開くセッションひとつ分の指定である。
type Request struct {
	Kind terminal.Kind
	// Alias は SSH のセッションの接続名である。シェルでは使わない。
	Alias string
	// ShellProfile は、シェルのプロファイルの ID である。nil なら設定の既定を使う。
	ShellProfile *string
	// WorkingDirectory は、始める場所である。nil なら、SSH は接続先の既定、シェルは
	// StartDirectory の場所で始める。
	WorkingDirectory *string
	// Workspace は、セッションを入れるワークスペースの ID である。呼び出し側が
	// 有るものを確かめて渡す。
	Workspace string
	// Size がゼロなら、terminal.Registry の既定の寸法になる。
	Size terminal.Size
}

// Opener は、セッションを組み立てて Registry で開く。
type Opener struct {
	Registry *terminal.Registry
	// Connect が nil なら、SSH のセッションは開けない。
	Connect Connector
	// Shell はローカルシェルの絶対パスを解決する。ShellProfiles が nil のときだけ使う。
	Shell func() (string, error)
	// ShellProfiles は、このマシンで見つけて確かめたシェルだけを返す。
	ShellProfiles func() []platform.ShellProfile
	// DefaultShellProfile は、保存した既定の ID を返す。空ならこのマシンの既定である。
	DefaultShellProfile func() string
	// Environment は、セッションが継ぐ環境である。これは利用者が自分で行った
	// であろう接続なので、検査が使う最小環境ではなくユーザー本人の環境を継ぐ。
	Environment func() []string
	// StartDirectory は、ローカルシェルが始まる場所を返す。nil ならこのプロセスの
	// 作業ディレクトリを継ぐ。関数なのは、設定が動いている最中に変わるからである。
	StartDirectory func() string
	// Startup は、alias に割り当てた起動スニペットを、この接続で送るかを決める。
	Startup func(alias string) StartupSnippet
	// Connected は、SSH のセッションがつながったときに呼ぶ。
	Connected func(alias string)
	// RequireWorkspace は、ワークスペースが有るかを確かめる。Reopen が、消えた
	// ワークスペースを default に置き換えるのに使う。
	RequireWorkspace func(id string) error
	// ReconnectError と ReconnectStopNotice は terminal.Spec の同じ名前の欄へ渡す。
	// 接続の失敗を通信形式の語へ変えるのは HTTP の層なので、呼び出し側が渡す。
	ReconnectError      func(err error) (retry bool, problem string)
	ReconnectStopNotice func(problem string) string
}

// Open は、request のセッションを開く。
func (o Opener) Open(ctx context.Context, request Request) (*terminal.Session, error) {
	spec, err := o.spec(request)
	if err != nil {
		return nil, err
	}
	spec.Workspace = request.Workspace
	return o.open(ctx, spec)
}

// Reopen は、保存してあったセッションを開き直す。保存してから変わったものは断らずに
// 手放す。消えたワークスペースは default に、無くなったフォルダは始める場所の既定に、
// このマシンにもう無いシェルのプロファイルは既定のシェルにする。
func (o Opener) Reopen(ctx context.Context, saved terminalrestore.Session) (*terminal.Session, error) {
	spec, err := o.reopeningSpec(saved)
	if err != nil {
		return nil, err
	}
	session, err := o.open(ctx, spec)
	if err != nil {
		return nil, err
	}
	if saved.PinnedTitle != "" {
		_ = session.Rename(saved.PinnedTitle)
	}
	return session, nil
}

// reopeningSpec は、saved から、今も使えるものだけで起動一式を組み立てる（Reopen）。
func (o Opener) reopeningSpec(saved terminalrestore.Session) (terminal.Spec, error) {
	request := Request{Kind: saved.Kind, Alias: saved.Alias}
	if saved.ShellProfile != "" {
		request.ShellProfile = &saved.ShellProfile
	}
	if saved.WorkingDirectory != "" {
		request.WorkingDirectory = &saved.WorkingDirectory
	}
	// 手放すたびに組み立て直す。プロファイルとフォルダの両方が無くなっていても、
	// 先に断ったほうを手放したあとで、もう一方も手放せる。
	spec, err := o.spec(request)
	for err != nil {
		switch {
		case request.WorkingDirectory != nil && (errors.Is(err, terminal.ErrInvalidRemoteWorkingDirectory) || errors.Is(err, terminal.ErrInvalidLocalWorkingDirectory)):
			request.WorkingDirectory = nil
		case request.ShellProfile != nil && errors.Is(err, platform.ErrUnknownShellProfile):
			request.ShellProfile = nil
		default:
			return terminal.Spec{}, err
		}
		spec, err = o.spec(request)
	}
	spec.Workspace = saved.Workspace
	if o.RequireWorkspace == nil || o.RequireWorkspace(saved.Workspace) != nil {
		spec.Workspace = terminalworkspace.DefaultID
	}
	return spec, nil
}

func (o Opener) open(ctx context.Context, spec terminal.Spec) (*terminal.Session, error) {
	session, err := o.Registry.Open(ctx, spec)
	if err != nil {
		return nil, err
	}
	if spec.Kind == terminal.KindSSH && o.Connected != nil {
		alias := spec.Alias
		session.WhenConnected(func() { o.Connected(alias) })
	}
	return session, nil
}

// spec は、開こうとしているセッションひとつ分の起動一式を組み立てる。
func (o Opener) spec(request Request) (terminal.Spec, error) {
	if request.Kind == terminal.KindShell {
		return o.shellSpec(request)
	}
	return o.sshSpec(request)
}

func (o Opener) sshSpec(request Request) (terminal.Spec, error) {
	target := request.Alias
	if target == "" {
		return terminal.Spec{}, ErrMissingAlias
	}
	if err := validate.Alias(target); err != nil {
		return terminal.Spec{}, err
	}
	if o.Connect == nil {
		return terminal.Spec{}, terminal.ErrNoStarter
	}
	initialDirectory := ""
	if request.WorkingDirectory != nil {
		var err error
		initialDirectory, err = terminal.CheckRemoteWorkingDirectory(*request.WorkingDirectory)
		if err != nil {
			return terminal.Spec{}, err
		}
	}

	// 設定を読むのはここである。読めなければセッションを作らない。設定の
	// 問題は接続画面が表示できるので、端末に理由を書く必要が無い。接続そのものの
	// 出来事（届かない、認証が通らない）は、開いたセッションの中で語られる。
	return terminal.Spec{
		Kind: terminal.KindSSH, Alias: target, Title: target, Size: request.Size,
		Open: func(ctx context.Context, size terminal.Size) (terminal.Process, error) {
			// 確保が取り消されたなら、繋ぎに行かない。
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// 成功したセッションの寿命は、要求から切り離す。Dialer.Open は
			// すぐ返り、渡された context を非同期の接続のあいだ持ち続けるので、
			// 要求の context をそのまま渡すと、開いた HTTP ハンドラが返った瞬間に
			// SSH セッションが終了する。取り消す権利は Process.Close と ForceClose が
			// 持つ（sshclient.Session は自分で派生させた context をそこで取り消す）。
			return o.Connect(context.WithoutCancel(ctx), target, size)
		},
		// 起動時のコマンドと送らなかったことの知らせは、terminal.Session が Ready の
		// 後に世代ごとに送る。再接続のたびに解決し直すので、割り当ての変更と停止がすぐ効く。
		Startup: func() terminal.Startup {
			commands := make([]string, 0, 2)
			if command, ok := terminal.ChangeRemoteDirectoryCommand(initialDirectory); initialDirectory != "" && ok {
				commands = append(commands, command)
			}
			var startup StartupSnippet
			if o.Startup != nil {
				startup = o.Startup(target)
			}
			if startup.Command != "" {
				commands = append(commands, startup.Command)
			}
			return terminal.Startup{Notice: startup.Notice, Commands: commands}
		},
		ReconnectError:      o.ReconnectError,
		ReconnectStopNotice: o.ReconnectStopNotice,
	}, nil
}

func (o Opener) shellSpec(request Request) (terminal.Spec, error) {
	profile, err := o.resolveShellProfile(request.ShellProfile)
	if err != nil {
		return terminal.Spec{}, err
	}
	directory := o.startDirectory()
	if request.WorkingDirectory != nil {
		directory, err = terminal.ResolveLocalWorkingDirectory(*request.WorkingDirectory)
		if err != nil {
			return terminal.Spec{}, err
		}
	}
	return terminal.Spec{
		Kind: terminal.KindShell, ShellProfile: profile.ID, Title: shellTitle(profile.Path), Size: request.Size,
		Command: terminal.Command{
			Path: profile.Path, Argv0: profile.Argv0,
			Arguments: append([]string(nil), profile.Arguments...), Env: o.environment(),
			Dir: directory,
		},
	}, nil
}

// shellTitle は、タブに出す名前である。
//
// filepath でよい。ここに来るのは、このマシンで解決されたローカルシェルの
// 絶対パスだけなので、区切り文字はこの OS のものである。自前で `/` だけを
// 数えると、Windows のタブには `C:\Windows\System32\...\powershell.exe` が
// まるごと並ぶ。
func shellTitle(shell string) string { return filepath.Base(shell) }

// ShellProfileChoices は、新しいシェルのメニューに出すプロファイルと、既定として
// 印を付ける ID を返す。保存した既定がこのマシンに無ければ、このマシンの既定に印を付ける。
func (o Opener) ShellProfileChoices() ([]platform.ShellProfile, string) {
	profiles := []platform.ShellProfile(nil)
	if o.ShellProfiles != nil {
		profiles = o.ShellProfiles()
	}
	chosen := "default"
	if o.DefaultShellProfile != nil && o.DefaultShellProfile() != "" {
		chosen = o.DefaultShellProfile()
	}
	if _, err := platform.ResolveShellProfile(profiles, chosen); err != nil {
		chosen = "default"
	}
	return profiles, chosen
}

func (o Opener) resolveShellProfile(requested *string) (platform.ShellProfile, error) {
	if o.ShellProfiles == nil {
		shell, err := o.resolveShell()
		if err != nil {
			return platform.ShellProfile{}, err
		}
		if requested != nil && *requested != "" && *requested != "default" {
			return platform.ShellProfile{}, platform.ErrUnknownShellProfile
		}
		return platform.ShellProfile{
			ID: "default", Label: shellTitle(shell), Path: shell,
			Argv0: platform.LoginArgv0(shell), Arguments: platform.LoginArguments(shell),
		}, nil
	}
	id := ""
	if requested != nil {
		id = *requested
	} else if o.DefaultShellProfile != nil {
		id = o.DefaultShellProfile()
	}
	profiles := o.ShellProfiles()
	profile, err := platform.ResolveShellProfile(profiles, id)
	if err != nil && requested == nil {
		// Metadata may be synced from another OS. An unavailable stored default
		// must not prevent the machine's verified login shell from opening.
		return platform.ResolveShellProfile(profiles, "default")
	}
	return profile, err
}

func (o Opener) resolveShell() (string, error) {
	if o.Shell == nil {
		return "", platform.ErrNoLoginShell
	}
	return o.Shell()
}

func (o Opener) startDirectory() string {
	if o.StartDirectory == nil {
		return ""
	}
	return o.StartDirectory()
}

func (o Opener) environment() []string {
	if o.Environment == nil {
		return nil
	}
	return o.Environment()
}
