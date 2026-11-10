package terminal

import (
	"context"
	"sync"
	"time"
)

// readChunk は、PTY から一度に読むバイト数である。
const readChunk = 32 << 10

// Session は、開かれている端末ひとつである。
type Session struct {
	id            string
	kind          Kind
	alias         string
	shellProfile  string
	title         string
	fallbackTitle string
	titleSource   TitleSource
	titlePinned   bool
	started       time.Time
	// workspace は、このセッションが入っているワークスペースの ID である。移せるので
	// mutex の中で読み書きする。
	workspace string

	mutex      sync.Mutex
	inputMutex sync.Mutex
	buffer     *Ring
	streams    map[*Stream]bool
	process    Process
	exited     *ExitInfo
	cleanup    func()
	state      State
	problem    string
	generation uint64
	ready      *processReadiness
	// screen は、バッファが手放した出力が残した画面である。求められたカーソルから流し直せないときに使う。
	// screen_checkpoint.go。
	screen screenCheckpoint
	// shell は、今のシェルが報告してきたことである。シェルが替わったら丸ごと捨てる。shell_reports.go。
	shell shellReports
	// runningCommandTitle は、ペインの名前に走っているコマンドを出してよいと設定が言っているかを返す。nil なら
	// 出してよい。running_command_title.go。
	runningCommandTitle func() bool
	// workingDirectory は、シェルが最後に報告したフォルダである。working_directory.go。
	workingDirectory    string
	notificationVersion uint64
	lastNotification    *Notification
	// programStatus は、OSC 7501 の記録を持つ。program_status_records.go。
	programStatus programStatusRecords
	// programBlocked は、セッションのプログラムが利用者を待ち始めたとき・待ち終えたときに知らされる。聞き手が
	// いなければ nil。blockedReported は、最後に知らせた内容である。blocked_program.go。
	programBlocked  func(BlockedProgramChange)
	blockedReported bool
	// screenPrompt は、問いを報告できないプログラムの画面から、問いを読み取る。screen_prompt_watch.go。
	screenPrompt screenPromptWatch
	// visible は、つながっているブラウザのどれかがこのセッションを見せているかもしれないかである。replies は、
	// プログラムへ向かう途中の返事を持つ。visibility_reports.go。
	visible bool
	replies chan []byte

	connectedCallback func()
	connectedOnce     sync.Once

	// tmuxSession は、このセッションの接続が開く tmux のセッションの名前である。接続が tmux を使うときだけ入る。
	// 変わることは無い。tmux_session.go。
	tmuxSession string
	// resumeProgram は、このセッションが失ったシェルで走っていたプログラム（OSC 7501 の名前）である。利用者が
	// 新しいシェルで何かを実行するまで持つ。resume_program.go。
	resumeProgram   string
	resumeDirectory string
	// resumeStartsInDirectory は、新しいシェルが、そのプログラムの走っていたフォルダで始まることを表す。
	resumeStartsInDirectory bool

	reopen         func(ctx context.Context, size Size) (Process, error)
	reconnectError func(error) (retry bool, problem string)
	stopNotice     func(problem string) string
	reopenFailure  func(error) string
	startup        func() Startup
	// log は、ターミナルに出たものを残す（session_log.go）。ログを残さないセッションでは nil。
	log OutputLog
	// logConnectedBefore は、ログの中で、再接続と最初の接続を見分ける。
	logConnectedBefore bool
	// logPumpGeneration は、シェルの出力そのものを渡せない process について、ログが pump から出力を追っている
	// 世代である。0 は追っていない。
	logPumpGeneration uint64
	// stopLogWatch は、ログが見ている世代のシェルの出力（ShellOutput）を追うのをやめる。
	stopLogWatch func()
	size         Size
	// reconnection は、自動再接続の進み具合である。reconnection.go。
	reconnection reconnection
	// network は、このマシンの回線について OS が言うことである。nil なら OS は何も言わない。
	network      *NetworkWatch
	networkRetry time.Duration
	now          func() time.Time
	// stopping は、繋ぎ直しを待っている最中に閉じられたことを伝える。
	stopping chan struct{}
	// discarded は、ユーザーが自分でこのコンソールを閉じたことを表す。
	discarded bool
	delay     func(attempt int) time.Duration
	// reconnectLimit は、自動再接続を何回まで試みてよいかを、試みるたびに返す。
	// 設定の変更を次の試みから効かせるため、値ではなく関数で持つ。
	reconnectLimit func() int

	// done は pump が終わったことを示す。テストと停止処理だけが待つ。
	done chan struct{}
}

type processReadiness struct {
	done        chan struct{}
	err         error
	connectedAt time.Time
	// userInitiated は、この世代を利用者の操作（開く、手動の再接続）が始めたことを
	// 表す。自動再接続の試みではないので、Ready の失敗を再接続の予算に数えない。
	userInitiated bool
}

// View は、一覧に出すためのセッションひとつ分である。
type View struct {
	ID        string
	Kind      Kind
	Alias     string
	Title     string
	Workspace string
	Started   time.Time
	Exited    *ExitInfo
	State     State
	Problem   string
	Reconnect *ReconnectView
	// Progress は、SSH の process が接続中か再接続中のあいだ入る。
	Progress *ConnectionProgress
	// HostKeyQuestion は、known_hosts に無いホスト鍵を信頼するかの答えを、接続が待っているあいだ入る。
	HostKeyQuestion *HostKeyQuestion
	// AwaitingPrompt は、接続が認証の問いへの答えを待っているかである。つながる前に Write が受け取る入力は
	// これだけである。
	AwaitingPrompt bool
	// PasswordSaveQuestion は、この接続に打ち込んだパスワードが、保存するかの答えを待っているあいだ真である。
	// パスワードそのものは View に入れない。
	PasswordSaveQuestion bool
	// Resume は、失ったシェルで走っていたプログラムを起動し直せるあいだ入る。
	Resume *ResumeHint
	// ProgramProgress は、今のプログラムの OSC 9;4 の報告である。SSH の接続の進み具合とは別である。
	ProgramProgress *ProgramProgress
	// ProgramStatus は、出す OSC 7501 の記録である。無ければ nil。
	ProgramStatus *ProgramStatus
	// Forwards は、このセッションが開いている転送である。
	Forwards     []Forward
	Presentation Presentation
	// ShellProfile は、シェルのセッションが使うローカルのシェルプロファイルである。
	ShellProfile string
	// WorkingDirectory は、セッションが今いるフォルダである。分からなければ ""。
	WorkingDirectory string
	// NotificationVersion は、プログラムが通知を求めるたびに増える。LastNotification は、いちばん新しい通知である。
	NotificationVersion uint64
	LastNotification    *Notification
}

// CommandTarget は、確認の時点で生きているターミナル1つを、サーバーが決めた形で指す。再接続が新しい Process を
// 入れるたびに、公開しているセッションの ID が同じままでも Generation は変わる。
type CommandTarget struct {
	ID         string
	Kind       Kind
	Alias      string
	Title      string
	Generation uint64
}

func (s *Session) ID() string { return s.id }

// WhenConnected は、非同期の SSH の Process が実際に認証を終え、リモートのシェルを始めたあとに、1度だけ走らせる
// 処理を登録する。Ready のあとに登録してもよい。接続の成功を記録する前に、ストリームのチケットを発行しなければ
// ならないためである。
func (s *Session) WhenConnected(callback func()) {
	if callback == nil {
		return
	}
	s.mutex.Lock()
	s.connectedCallback = callback
	connected := s.state == StateConnected
	s.mutex.Unlock()
	if connected {
		s.signalConnected()
	}
}

func (s *Session) signalConnected() {
	s.mutex.Lock()
	callback := s.connectedCallback
	connected := s.state == StateConnected
	s.mutex.Unlock()
	if connected && callback != nil {
		s.connectedOnce.Do(callback)
	}
}

// Exit は終了理由を返す。実行中の場合は nil を返す。
func (s *Session) Exit() *ExitInfo {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited == nil {
		return nil
	}
	info := *s.exited
	return &info
}

// Done は現在のprocess世代のpumpが終了したとき閉じる。返したchannelは、手動再接続が
// 新しい世代を開始しても元の世代を指し続けるため、呼び出し側は待ち始めた終了だけを
// 決定的に観測できる。
func (s *Session) Done() <-chan struct{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.done
}

func (s *Session) Live() bool { return s.Exit() == nil }

// View は一覧に出すための写しである。
func (s *Session) View() View {
	directory := s.WorkingDirectory()
	program, running := ForegroundProgram{}, false
	if s.showsRunningCommand() {
		program, running = s.foregroundProgram()
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	title := s.displayTitleLocked(program, running)
	view := View{
		ID: s.id, Kind: s.kind, Alias: s.alias, Title: title, Workspace: s.workspace,
		Started: s.started, State: s.state, Problem: s.problem,
		ShellProfile: s.shellProfile, WorkingDirectory: directory,
		Presentation:        Presentation{DisplayTitle: title, TitleSource: s.titleSource, TitlePinned: s.titlePinned},
		NotificationVersion: s.notificationVersion,
	}
	if s.lastNotification != nil {
		notification := *s.lastNotification
		view.LastNotification = &notification
	}
	if s.shell.programProgress != nil {
		progress := *s.shell.programProgress
		view.ProgramProgress = &progress
	}
	view.ProgramStatus = s.programStatus.summary()
	view.Resume = s.resumeHintLocked()
	if s.reconnection.view != nil {
		status := *s.reconnection.view
		view.Reconnect = &status
	}
	if s.exited != nil {
		info := *s.exited
		view.Exited = &info
	}
	if forwarder, ok := s.process.(Forwarder); ok {
		view.Forwards = forwarder.Forwards()
	}
	if progressing, ok := s.process.(Progressing); ok &&
		(s.state == StateConnecting || s.state == StateReconnecting) {
		progress := progressing.ConnectionProgress()
		if progress.Phase != "" {
			view.Progress = &progress
		}
	}
	if questioning, ok := s.process.(HostKeyQuestioning); ok &&
		(s.state == StateConnecting || s.state == StateReconnecting) {
		if question, asking := questioning.HostKeyQuestion(); asking {
			view.HostKeyQuestion = &question
		}
	}
	if prompting, ok := s.process.(Prompting); ok &&
		(s.state == StateConnecting || s.state == StateReconnecting) {
		view.AwaitingPrompt = prompting.AwaitingPrompt()
	}
	if asking, ok := s.process.(PasswordSaveAsking); ok && s.state == StateConnected {
		_, view.PasswordSaveQuestion = asking.TypedPassword()
	}
	return view
}

// Write は打鍵を PTY へ渡す。
func (s *Session) Write(p []byte) (int, error) {
	s.inputMutex.Lock()
	defer s.inputMutex.Unlock()
	s.mutex.Lock()
	process, exited, state := s.process, s.exited, s.state
	s.mutex.Unlock()
	if exited != nil || process == nil {
		if exited == nil && (state == StateConnecting || state == StateReconnecting) {
			return len(p), nil
		}
		return 0, ErrExited
	}
	if state == StateConnecting || state == StateReconnecting {
		prompting, ok := process.(Prompting)
		if !ok || !prompting.AwaitingPrompt() {
			// Ready前の通常入力を溜めない。明示的な認証promptだけが例外である。
			return len(p), nil
		}
		return process.Write(p)
	}
	s.forgetProgramToResumeOnEnter(p)
	return process.Write(p)
}

// AnswerHostKey は、接続が待っているホスト鍵の問いに答える。問いが、まだこの fingerprint の鍵についてのもので
// あるときだけ答える。利用者が打ち込んで答えたあとや、接続が先へ進んだあとに押したボタンは、何にも答えない。
// 答えはその問いにだけ書き、あとの問いやシェルには書かない。
func (s *Session) AnswerHostKey(fingerprint string, accept bool) error {
	s.inputMutex.Lock()
	defer s.inputMutex.Unlock()
	s.mutex.Lock()
	process, state := s.process, s.state
	s.mutex.Unlock()
	questioning, ok := process.(HostKeyQuestioning)
	if !ok || (state != StateConnecting && state != StateReconnecting) {
		return ErrNoHostKeyQuestion
	}
	if !questioning.AnswerHostKey(fingerprint, accept) {
		return ErrNoHostKeyQuestion
	}
	return nil
}

// AnswerPasswordSave は、この接続に打ち込んだパスワードを保存するかに答える。answer が保存するか断るかを
// 行う。断られた答え（Vault がロックされている）は問いを残すので、利用者は答え直せる。
func (s *Session) AnswerPasswordSave(answer func(TypedPassword) error) error {
	s.mutex.Lock()
	asking, ok := s.process.(PasswordSaveAsking)
	s.mutex.Unlock()
	if !ok {
		return ErrNoPasswordSaveQuestion
	}
	typed, waiting := asking.TypedPassword()
	if !waiting {
		return ErrNoPasswordSaveQuestion
	}
	if err := answer(typed); err != nil {
		return err
	}
	asking.ForgetTypedPassword()
	return nil
}

// CommandTarget は、正確な入力のできる、接続済みの Process についてだけ、指す先を返す。一斉送信のプレビューは
// 接続先の名前ではなくこれを使うので、代わりのターミナルを黙って開くことが無い。
func (s *Session) CommandTarget() (CommandTarget, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.exited != nil || s.process == nil || s.state != StateConnected {
		return CommandTarget{}, ErrNotConnected
	}
	if _, ok := s.process.(ExactInput); !ok {
		return CommandTarget{}, ErrExactInputUnavailable
	}
	alias := s.alias
	if alias == "" && s.kind == KindShell {
		alias = "localhost"
	}
	return CommandTarget{ID: s.id, Kind: s.kind, Alias: alias, Title: s.title, Generation: s.generation}, nil
}

// WriteCommandInput は、プレビューが捉えた世代の Process にだけ書く。submit が false なら、復帰（CR）を
// 付けずにバイト列を入れる。
func (s *Session) WriteCommandInput(ctx context.Context, generation uint64, command string, submit bool) error {
	if len(command) == 0 || len(command) > MaxCommandBytes {
		return ErrCommandTooLarge
	}
	extra := 0
	if submit {
		extra = 1
	}
	payload := make([]byte, len(command)+extra)
	copy(payload, command)
	if submit {
		payload[len(command)] = '\r'
	}

	s.inputMutex.Lock()
	defer s.inputMutex.Unlock()
	s.mutex.Lock()
	if s.generation != generation {
		s.mutex.Unlock()
		return ErrGenerationChanged
	}
	process, exited, state := s.process, s.exited, s.state
	s.mutex.Unlock()
	if exited != nil || process == nil || state != StateConnected {
		return ErrNotConnected
	}
	writer, ok := process.(ExactInput)
	if !ok {
		return ErrExactInputUnavailable
	}
	s.forgetProgramToResumeOnEnter(payload)
	return writer.WriteExact(ctx, payload)
}

// Resize は PTY の大きさを変え、その大きさを覚える。再接続や置き換えで開く
// 新しい PTY はこの最新の大きさで始まる。再接続を待っている間（PTY がまだ
// 無い間）の変更も覚える。ブラウザは一度送った大きさを送り直さないので、
// ここで捨てると再接続後の PTY が古い大きさになり、リモートのプログラムが
// 見えない行へ描く。
func (s *Session) Resize(size Size) error {
	if !size.Valid() {
		return ErrInvalidSize
	}
	s.mutex.Lock()
	process, exited := s.process, s.exited
	log := s.log
	if exited == nil {
		s.size = size
		s.screen.resized(s.buffer.Written(), size)
		s.resizeFollowedScreenLocked(size)
	}
	s.mutex.Unlock()
	if log != nil && exited == nil {
		log.Resize(size)
	}
	if exited != nil {
		return ErrExited
	}
	if process == nil {
		// 再接続待ち。次の PTY は覚えた大きさで開く。
		return nil
	}
	return process.Resize(size)
}

// Discard は、このセッションがユーザーの意思で閉じられたことを記録する。
func (s *Session) Discard() {
	s.mutex.Lock()
	s.discarded = true
	s.mutex.Unlock()
}

// Discarded は、ユーザーが自分で閉じたかを返す。
func (s *Session) Discarded() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.discarded
}

// Hangup は子プロセスへ SIGHUP を送る。終了そのものは pump が観測する。
func (s *Session) Hangup() error {
	s.stopReconnecting()
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	return process.Hangup()
}

// stoppedLocked は、このセッションの process をもう公開しないと決まったかを返す。
// 利用者が閉じたか（discarded）、再接続を止めたか engine が止まるか（stopping）で決まる。
// 呼び出し側が mutex を持つ。
func (s *Session) stoppedLocked() bool {
	if s.discarded {
		return true
	}
	select {
	case <-s.stopping:
		return true
	default:
		return false
	}
}

// forceClose は、自動再接続を止め、生きている Process を強制停止する。
func (s *Session) forceClose() error {
	s.stopReconnecting()
	s.mutex.Lock()
	process, exited := s.process, s.exited
	s.mutex.Unlock()
	if exited != nil || process == nil {
		return nil
	}
	return process.ForceClose()
}

// observeProcess は、今の Process が使えるようになるのを見張り始める。pending は、Ready が成功するまで
// 見せる状態である。Ready を持たない Process は、Open が同期で済むものとして扱う。
func (s *Session) observeProcess(pending State, successMessage string) {
	s.mutex.Lock()
	s.noteProgramToResumeLocked()
	// 前のシェルはもう無い。それが報告したこと（名前、進み具合、走っているコマンド、可視の報告の求め）は
	// 新しいシェルのものではない。
	s.forgetShellReportsLocked()
	// 前のプロセスがしていたことも終わった。done や failed で残した結果は、ブラウザが見るまで残す。
	s.endActiveProgramStatusLocked()
	s.programStatus.drop(ProgramStatusIdle)
	s.screenPrompt.stop()
	s.generation++
	generation := s.generation
	// 前のシェルは既に無い。ここから先の入力は新しいシェルへ届く。
	s.announceShellReplacedLocked()
	process := s.process
	readier, asynchronous := process.(Readier)
	if !asynchronous {
		s.ready = nil
		s.state = StateConnected
		s.problem = ""
		s.reconnection.view = nil
		s.mutex.Unlock()
		s.announceConnected(process, generation, successMessage)
		return
	}
	observed := &processReadiness{done: make(chan struct{}), userInitiated: pending == StateConnecting}
	s.ready = observed
	s.state = pending
	s.mutex.Unlock()

	go func() {
		<-readier.Ready()
		err := readier.ReadyErr()
		observed.err = err
		if err == nil {
			observed.connectedAt = s.now()
		}
		s.mutex.Lock()
		stopped := s.stoppedLocked() || s.exited != nil
		current := s.generation == generation && s.ready == observed && !stopped
		if current && err == nil {
			s.state = StateConnected
			s.problem = ""
			s.reconnection.view = nil
		}
		s.mutex.Unlock()
		close(observed.done)
		if current && err == nil {
			s.announceConnected(process, generation, successMessage)
		}
	}()
}

// announceConnected は、Process が使える状態になったことをターミナルへ書き、起動時のものを送る。
//
// 動いていた tmux のセッションに戻ったなら、前のプログラムはそこで動いている。新しいシェルとは
// 言わず、続きからの案内も、打ち込む起動時のコマンドも要らない。
func (s *Session) announceConnected(process Process, generation uint64, successMessage string) {
	resumed := resumedTmuxSession(process)
	if resumed {
		s.mutex.Lock()
		s.resumeProgram, s.resumeDirectory = "", ""
		s.mutex.Unlock()
	}
	if successMessage != "" {
		if resumed {
			successMessage = reattachedTmuxMessage
		}
		s.publish([]byte(successMessage))
	}
	s.announceProgramToResume()
	s.signalConnected()
	s.startLog(process, generation)
	if !resumed {
		s.sendStartup(process)
	}
}

// reattachedTmuxMessage は、再接続で動いていた tmux のセッションに戻ったときに書く行である。
const reattachedTmuxMessage = "\r\n[sshc] 再接続しました。\r\n"

// sendStartup は、Spec.Startup の知らせと手順を、使える状態になった Process へ送る。
//
// 世代ごとに呼ぶので、再接続した新しいシェルにも同じものが届く。SSH の Process では
// Ready を待ってから送るので、手順は認証の問いへ答えとして流れず、知らせも
// 認証の問いに紛れない。待つ文字列のある手順を含むときは、待つあいだ呼び手を止めない
// よう、別の goroutine で送る（runStartupSteps）。
func (s *Session) sendStartup(process Process) {
	if s.startup == nil {
		return
	}
	startup := s.startup()
	if announcer, ok := process.(Announcer); ok && startup.Notice != "" {
		announcer.Announce(startup.Notice)
	}
	s.hideInLog(startup.Secrets)
	if !startupStepsWait(startup.Steps) {
		for _, step := range startup.Steps {
			_, _ = process.Write(step.payload())
		}
		return
	}
	s.mutex.Lock()
	generation := s.generation
	s.mutex.Unlock()
	go s.runStartupSteps(process, generation, startup)
}

// startPump は、セッションを動かし始める。回線の知らせは、呼び出し側へ戻る前に受け取り始める。
// セッションが呼び出し側の手に渡ったあとの回線の変化を、取りこぼさないためである。
func (s *Session) startPump(now func() time.Time) {
	_, networkChanged := s.network.State()
	go s.watchNetworkChanges(networkChanged, s.done)
	go s.pump(now)
}

// pump は PTY を読み、バッファへ書き、アタッチしているものへ配る。
func (s *Session) pump(now func() time.Time) {
	defer close(s.done)
	go s.writeReplies(s.done)
	connectedAt := s.started
	for {
		s.mutex.Lock()
		process, ready, generation := s.process, s.ready, s.generation
		s.mutex.Unlock()
		// commandTimes は、読んでいるかたまりの中で見たコマンドの開始と終了である（command_times.go）。
		var commandTimes []commandTime
		observer := newOSCObserver(oscReports{
			Title:         func(title string) { s.acceptTitle(generation, title) },
			Notify:        func(title, body string) { s.acceptNotification(generation, title, body, now()) },
			Progress:      func(progress *ProgramProgress) { s.acceptProgramProgress(generation, progress) },
			Directory:     func(directory string) { s.acceptWorkingDirectory(generation, directory) },
			PromptStarted: func() { s.acceptPromptStarted(generation) },
			CommandStarted: func() {
				at := now()
				s.acceptCommandStarted(generation, at)
				commandTimes = append(commandTimes, commandTime{kind: 'C', at: at})
			},
			CommandFinished: func(exitCode *int) {
				at := now()
				s.acceptCommandFinished(generation, exitCode, at)
				commandTimes = append(commandTimes, commandTime{kind: 'D', at: at})
			},
			ProgramStatus: func(report programStatusReport) { s.acceptProgramStatus(generation, report) },
			Reset:         func() { s.acceptTerminalReset(generation) },
		})
		visibilityRequests := newVisibilityRequestObserver(func(request visibilityRequest) { s.acceptVisibilityRequest(generation, request) })
		alternateScreen := newAlternateScreenObserver(func() { s.startFollowingScreen(generation) })
		buffer := make([]byte, readChunk)
		for {
			read, err := process.Read(buffer)
			if read > 0 {
				observer.Observe(buffer[:read])
				visibilityRequests.Observe(buffer[:read])
				alternateScreen.Observe(buffer[:read])
				s.followScreenOutput(generation, buffer[:read])
				s.publish(buffer[:read])
				s.logPumpOutput(generation, buffer[:read])
				if len(commandTimes) > 0 {
					s.publish(commandTimesSequence(commandTimes))
					commandTimes = commandTimes[:0]
				}
			}
			if err != nil {
				break
			}
		}
		// このプロセスはもう出力しない。次のシェルの出力より前にモードを戻す。
		s.publish([]byte(leftoverModeReset))
		info := process.Wait()
		// 終わり方の文は、戻したあとの通常の画面へ書く。
		if info.Notice != "" {
			s.publish([]byte(info.Notice))
		}
		var connectionErr error
		if ready != nil {
			<-ready.done
			connectionErr = ready.err
		}
		s.endLogOfProcess(connectionErr == nil)
		if info.At.IsZero() {
			info.At = now()
		}
		_ = process.Close()
		settledAt := connectedAt
		if ready != nil && !ready.connectedAt.IsZero() {
			settledAt = ready.connectedAt
		}
		// 握手に長く掛かった時間は安定稼働に数えない。Ready に失敗した
		// 再接続で予算を戻すと、失敗し続ける接続が永久に回り続ける。
		if connectionErr == nil && info.At.Sub(settledAt) >= ReconnectSettled {
			s.mutex.Lock()
			s.reconnection.attempts = 0
			s.mutex.Unlock()
		}

		if connectionErr != nil && ready.userInitiated {
			// 一度も繋がっていない接続を「切れました」として試し直さない。
			// 失敗の理由は接続ログとしてターミナルに出ている。
			s.recordConnectionFailure(connectionErr)
			s.announceEnterToReconnect(info)
			s.finish(info)
			return
		}
		if !s.reconnect(info, connectionErr, now) {
			s.announceEnterToReconnect(info)
			s.finish(info)
			return
		}
		connectedAt = now()
	}
}

// problemConnectFailed は、利用者が始めた接続が、固定の problem code を持たない
// 理由（名前解決、接続の拒否、タイムアウトなど）で失敗したことを表す。
const problemConnectFailed = "connect_failed"

// recordConnectionFailure は、利用者が始めた接続の失敗を problem に残す。
// 自動再接続はしていないので、再接続を止めたという文は書かない。
func (s *Session) recordConnectionFailure(err error) {
	problem := s.connectionFailureProblem(err)
	s.mutex.Lock()
	s.problem = problem
	s.mutex.Unlock()
}

// connectionFailureProblem は、利用者が始めた接続（最初の接続と手動の再接続）の
// 失敗を problem code にする。自動で試し直さないので、理由の分からない失敗は
// reconnect_failed ではなく connect_failed で表す。Ready の前に開けなかったときも、
// Ready で失敗したときも同じ code にする。
func (s *Session) connectionFailureProblem(err error) string {
	if _, code := s.classifyReconnectFailure(err); code != ProblemReconnectFailed {
		return code
	}
	return problemConnectFailed
}

// abandonProcess は、公開しないと決めた process を強制停止して閉じる。
// 終了の観測は呼び出し側が行う。
func abandonProcess(process Process) {
	_ = process.ForceClose()
	_ = process.Close()
}

// abandonAndWait は、pump が観測しない process を捨て、終わるまで待つ。
func abandonAndWait(process Process) {
	abandonProcess(process)
	process.Wait()
}

func (s *Session) finish(info ExitInfo) {
	s.mutex.Lock()
	if s.exited != nil {
		s.mutex.Unlock()
		return
	}
	log, stopLogWatch := s.takeLogLocked()
	s.finishLocked(info)
	s.mutex.Unlock()
	closeLog(log, stopLogWatch)
}

// finishLocked は、セッションの終わり方を記録し、持っていたものを手放す。呼び出し側が mutex を持つ。
func (s *Session) finishLocked(info ExitInfo) {
	s.exited = &info
	s.state = StateExited
	s.reconnection.view = nil
	s.screenPrompt.stop()
	s.reportBlockedProgramLocked()
	s.closeStreamsLocked()
	if s.cleanup != nil {
		cleanup := s.cleanup
		s.cleanup = nil
		cleanup()
	}
}
