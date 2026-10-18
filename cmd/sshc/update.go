package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"sshc/internal/releasecheck"
)

const (
	latestReleaseAPI = "https://api.github.com/repos/aida0710/sshc/releases/latest"
	maxInstallerSize = 1 << 20

	// updateReleaseCheckTimeout は、sshc update が最新リリースを尋ねる上限である。
	// 利用者は確認そのものを求めて待っているので、engine の起動時の確認
	// （releaseCheckTimeout）より長く、遅い回線でも答えを待つ。
	updateReleaseCheckTimeout = 30 * time.Second
	// installerDownloadTimeout は、installer（maxInstallerSize 以下のスクリプト）1 本を
	// 取得する上限である。本体の取得は installer 自身が行うので、ここには含まれない。
	installerDownloadTimeout = 30 * time.Second
	// Tagged installer downloads, including redirects, stay on this HTTPS host.
	taggedInstallerHost = "raw.githubusercontent.com"
)

// errHomebrewTapNotRefreshed は、brew upgrade が成功で終わったのに、新しい版が
// 入らなかったことを表す。Homebrew は tap を更新しないまま upgrade すると
// （HOMEBREW_NO_AUTO_UPDATE を設定している、数分以内に更新したばかり）、新しい
// formula を知らないので、今の版を最新とみなして成功で終わる。
var errHomebrewTapNotRefreshed = errors.New("Homebrew may not have refreshed " + homebrewTap +
	", for example because HOMEBREW_NO_AUTO_UPDATE is set")

type updateDependencies struct {
	executable        func() (string, error)
	detect            func(string) (installation, error)
	latest            func(context.Context) (releasecheck.Release, error)
	install           func(context.Context, installation, releasecheck.Release, io.Writer, io.Writer) error
	installWeb        func(context.Context, installation, releasecheck.Release) error
	upgradeHomebrew   func(context.Context, installation) (string, error)
	serviceExecutable func(context.Context, installation) (string, error)
	restartService    func(context.Context, string) (bool, error)
	// engineStatus は、再起動した engine の Vault の状態を読み、次にすることを選ぶために使う。
	engineStatus engineStatusReader
	confirm      actionConfirmer
}

func defaultUpdateDependencies() updateDependencies {
	checker := &releasecheck.Checker{
		API:  latestReleaseAPI,
		HTTP: &http.Client{Timeout: updateReleaseCheckTimeout},
	}
	installerClient := taggedInstallerHTTPClient()
	commands := systemInstallationCommands{}
	return updateDependencies{
		executable: os.Executable,
		detect:     detectInstallation,
		latest:     checker.Latest,
		install: func(ctx context.Context, found installation, release releasecheck.Release, stdout, stderr io.Writer) error {
			installer := updateInstaller{client: installerClient, commands: commands, stdout: stdout, stderr: stderr}
			return installer.install(ctx, found, release)
		},
		installWeb: func(ctx context.Context, found installation, release releasecheck.Release) error {
			installer := updateInstaller{client: installerClient, commands: commands, stdout: io.Discard, stderr: io.Discard, caller: "web-update"}
			return installer.install(ctx, found, release)
		},
		serviceExecutable: func(ctx context.Context, found installation) (string, error) {
			return managedInstallationExecutable(ctx, found, commands)
		},
		upgradeHomebrew: (updateInstaller{commands: commands, stdout: io.Discard, stderr: io.Discard}).upgradeHomebrewFromWeb,
		restartService:  restartManagedServiceAfterUpdate,
		engineStatus:    readEngineStatus,
		confirm:         systemActionConfirmer,
	}
}

// updateRun は、`sshc update` の 1 回の実行である。
type updateRun struct {
	// current は、走っている sshc の版である。
	current string
	// yes は、更新の確認を省く。
	yes bool
	// home は、再起動した service の engine を handoff から見つけるために使う。
	home         string
	stdout       io.Writer
	stderr       io.Writer
	dependencies updateDependencies
}

// updatePlan は、確認のあとで行う更新の中身である。
type updatePlan struct {
	found installation
	// latest の Version は、安定版の tag にそろえてある。
	latest releasecheck.Release
	// serviceExecutable は、管理された service に登録する実行ファイルである。
	// service を再起動しないときは空である。
	serviceExecutable string
}

// runUpdate は、管理元と最新の版を確かめ、確認のあとで新しい版を入れ、管理された
// service が動いていれば再起動する。各段は、続けられなければ終える終了コードを返す。
func runUpdate(ctx context.Context, run updateRun) int {
	found, code := run.findInstallation()
	if code != 0 {
		return code
	}
	latest, code := run.latestRelease(ctx)
	if code != 0 {
		return code
	}
	if !releasecheck.Newer(run.current, latest.Version) {
		fmt.Fprintf(run.stdout, "sshc: %s is already the latest release\n", run.current)
		return 0
	}
	serviceExecutable, code := run.registeredServiceExecutable(ctx, found)
	if code != 0 {
		return code
	}
	plan := updatePlan{found: found, latest: latest, serviceExecutable: serviceExecutable}
	if confirmed, code := run.confirm(ctx, plan); !confirmed {
		return code
	}
	if code := run.install(ctx, plan); code != 0 {
		return code
	}
	return run.restartAfterInstall(ctx, plan)
}

// findInstallation は、この実行ファイルを入れた管理元を見つける。管理元が分からない
// 実行ファイルは、sshc update では入れ替えない。
func (run updateRun) findInstallation() (installation, int) {
	executable, err := run.dependencies.executable()
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: find this executable: %v\n", err)
		return installation{}, exitFailure
	}
	found, err := run.dependencies.detect(executable)
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: inspect this installation: %v\n", err)
		return installation{}, exitFailure
	}
	if found.manager == managerUnknown {
		fmt.Fprint(run.stderr, unmanagedInstallationNotice(executable, runtime.GOOS))
		return installation{}, exitFailure
	}
	return found, 0
}

// latestRelease は、最新のリリースを尋ね、その版を安定版の tag にそろえて返す。
func (run updateRun) latestRelease(ctx context.Context) (releasecheck.Release, int) {
	latest, err := run.dependencies.latest(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return releasecheck.Release{}, exitInterrupted
		}
		if errors.Is(err, releasecheck.ErrNoRelease) {
			fmt.Fprintln(run.stderr, "sshc: no release is available")
		} else {
			fmt.Fprintf(run.stderr, "sshc: check the latest release: %v\n", err)
		}
		return releasecheck.Release{}, exitFailure
	}
	tag, ok := releasecheck.StableTag(latest.Version)
	if !ok {
		fmt.Fprintf(run.stderr, "sshc: the latest release has an invalid version %q\n", latest.Version)
		return releasecheck.Release{}, exitFailure
	}
	latest.Version = tag
	return latest, 0
}

// registeredServiceExecutable は、管理された service を再起動するときに登録する
// 実行ファイルを、入れ替える前に決める。service を再起動しないなら空を返す。
func (run updateRun) registeredServiceExecutable(ctx context.Context, found installation) (string, int) {
	if found.manager == managerPowerShell || run.dependencies.restartService == nil {
		return "", 0
	}
	if run.dependencies.serviceExecutable == nil {
		fmt.Fprintln(run.stderr, "sshc: update cannot identify the executable registered with the managed service")
		return "", exitFailure
	}
	executable, err := run.dependencies.serviceExecutable(ctx, found)
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: identify the executable registered with the managed service: %v\n", err)
		return "", exitFailure
	}
	return executable, 0
}

// confirm は、何をどう入れ替えるかを出し、進めてよいかを尋ねる。
func (run updateRun) confirm(ctx context.Context, plan updatePlan) (bool, int) {
	manager := "sshc's " + plan.found.manager.installerName()
	if plan.found.manager == managerHomebrew {
		manager = "Homebrew"
	}
	fmt.Fprintf(run.stdout, "sshc: update %s from %s to %s using %s\n",
		plan.found.executable, run.current, plan.latest.Version, manager)
	if plan.found.manager == managerPowerShell {
		fmt.Fprintln(run.stdout, "sshc: a running engine will restart and a password-protected vault will lock")
	} else if run.dependencies.restartService != nil {
		fmt.Fprintln(run.stdout, "sshc: if its managed service is active, it will restart and a password-protected vault will lock")
	}
	return confirmChange(ctx, changeConfirmation{
		yes: run.yes, confirmer: run.dependencies.confirm, stdout: run.stdout, stderr: run.stderr,
	})
}

// install は、見つけた管理元で新しい版を入れる。
func (run updateRun) install(ctx context.Context, plan updatePlan) int {
	fmt.Fprintf(run.stdout, "sshc: updating %s to %s\n", run.current, plan.latest.Version)
	if err := run.dependencies.install(ctx, plan.found, plan.latest, run.stdout, run.stderr); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		fmt.Fprintf(run.stderr, "sshc: update failed: %v\n", err)
		if errors.Is(err, errHomebrewTapNotRefreshed) {
			fmt.Fprintln(run.stderr, "sshc: run `brew update`, then `sshc update` again")
		}
		return exitFailure
	}
	fmt.Fprintf(run.stdout, "sshc: updated to %s\n", plan.latest.Version)
	return 0
}

// restartAfterInstall は、管理された service が動いていれば新しい版で再起動し、
// そうでなければ、手で起動した engine を再起動するよう案内する。service の定義が
// 以前の版の sshc が書いた形のままなら、再起動せずに sshc service install を案内する。
func (run updateRun) restartAfterInstall(ctx context.Context, plan updatePlan) int {
	// install.ps1 owns the stop/start pair, including engines outside Task Scheduler.
	if plan.found.manager == managerPowerShell {
		return 0
	}
	if run.dependencies.restartService != nil {
		restarted, err := run.dependencies.restartService(ctx, plan.serviceExecutable)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return exitInterrupted
			}
			if errors.Is(err, errOutdatedServiceDefinition) {
				fmt.Fprintf(run.stdout, "sshc: %s\n", outdatedServiceDefinitionAdvice)
				return 0
			}
			fmt.Fprintf(run.stderr, "sshc: update succeeded, but restart the managed service: %v\n", err)
			fmt.Fprintln(run.stderr, "sshc: run `sshc service install` to retry the service restart")
			return exitFailure
		}
		if restarted {
			fmt.Fprintln(run.stdout, "sshc: managed service restarted")
			if advice := vaultNextStep(ctx, run.dependencies.engineStatus, run.home); advice != "" {
				fmt.Fprintf(run.stdout, "sshc: %s\n", advice)
			}
			return 0
		}
	}
	fmt.Fprintln(run.stdout, "sshc: to use the new version, restart any engine outside the managed service with `sshc engine --replace`")
	return 0
}

// updateInstaller は、見つけた管理方法で新しい版を入れ、入った実行ファイルが
// その版を名乗ることまで確かめる。
type updateInstaller struct {
	// client fetches installers from the selected release tag.
	client   *http.Client
	commands installationCommands
	stdout   io.Writer
	stderr   io.Writer
	caller   string
}

func (installer updateInstaller) install(ctx context.Context, found installation, release releasecheck.Release) error {
	switch found.manager {
	case managerHomebrew:
		return installer.upgradeHomebrew(ctx, found, release.Version)
	case managerShell, managerPowerShell:
		return installer.runTaggedInstaller(ctx, found, release.Version)
	default:
		return errors.New("unsupported installation manager")
	}
}

func (installer updateInstaller) upgradeHomebrew(ctx context.Context, found installation, tag string) error {
	managedPath, err := installer.runHomebrewUpgrade(ctx, found)
	if err != nil {
		return err
	}
	if err := installer.verifyReportedVersion(ctx, managedPath, tag); err != nil {
		if errors.As(err, new(unexpectedVersionError)) {
			err = fmt.Errorf("%w; %w", err, errHomebrewTapNotRefreshed)
		}
		return fmt.Errorf("verify the upgraded Homebrew executable: %w", err)
	}
	return nil
}

// verifyReportedVersion は、入れ終えた実行ファイル自身に版を尋ね、tag と同じかを確かめる。
func (installer updateInstaller) verifyReportedVersion(ctx context.Context, executable, tag string) error {
	line, err := installer.commands.Output(ctx, executable, "version")
	if err != nil {
		return err
	}
	if !reportsVersion(line, tag) {
		return unexpectedVersionError{executable: executable, tag: tag}
	}
	return nil
}

// unexpectedVersionError は、入れ終えた実行ファイルが tag と違う版を名乗ったことを表す。
// 版を尋ねられなかった失敗と分けるのは、Homebrew ではこれが tap の古さを示すからである。
type unexpectedVersionError struct {
	executable string
	tag        string
}

func (failure unexpectedVersionError) Error() string {
	return fmt.Sprintf("%s does not report version %s", failure.executable, failure.tag)
}

// taggedInstallerHTTPClient は、tag 固定の install.sh の取得に使う client を作る。
func taggedInstallerHTTPClient() *http.Client {
	return &http.Client{Timeout: installerDownloadTimeout, CheckRedirect: allowTaggedInstallerRedirect}
}

// allowTaggedInstallerRedirect は、https の taggedInstallerHost への redirect だけを許す。
// Go の既定の client は、別の host への redirect にも http への格下げにも従う。
func allowTaggedInstallerRedirect(request *http.Request, _ []*http.Request) error {
	if request.URL.Scheme != "https" || request.URL.Hostname() != taggedInstallerHost {
		return errors.New("the tagged installer redirected outside https://" + taggedInstallerHost)
	}
	return nil
}

func reportsVersion(line []byte, tag string) bool {
	reported, ok := reportedReleaseVersion(line)
	return ok && reported == tag
}

func replaceEnvironment(current []string, replacements map[string]string) []string {
	replacedNames := make(map[string]bool, len(replacements))
	for name := range replacements {
		if runtime.GOOS == "windows" {
			name = strings.ToUpper(name)
		}
		replacedNames[name] = true
	}
	next := make([]string, 0, len(current)+len(replacements))
	for _, entry := range current {
		name, _, found := strings.Cut(entry, "=")
		if runtime.GOOS == "windows" {
			name = strings.ToUpper(name)
		}
		if found && replacedNames[name] {
			continue
		}
		next = append(next, entry)
	}
	for name, value := range replacements {
		next = append(next, name+"="+value)
	}
	return next
}
