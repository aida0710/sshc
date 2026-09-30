package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/connectionlog"
	"sshc/internal/keys"
	"sshc/internal/terminal"
	"sshc/internal/totp"
)

// ErrNoIdentity は、公開鍵認証に使える鍵がひとつも無いことを報告する。
var ErrNoIdentity = errors.New("no identity file and no agent key is available")

// maxPassphraseAttempts は、ひとつの鍵についてパスフレーズを尋ねる回数である。
//
// OpenSSH と同じ 3 回。上限が無いと、間違え続けるユーザーがこの接続を保持し続ける。
const maxPassphraseAttempts = 3

// maxPasswordAttempts は、OpenSSH の NumberOfPasswordPrompts と同じ再試行回数。
const maxPasswordAttempts = 3

// AgentConnector は、ssh-agent の宛先と開き方である。
//
// 宛先の決め方は OS ごとに違う。Unix は SSH_AUTH_SOCK の unix socket、Windows は
// OpenSSH の固定の named pipe である。ここでは決めず、Keys 画面の agent と同じ
// keys.Agent を受け取る。接続の公開鍵認証、agent 転送、Keys 画面の登録が同じ
// agent を指すようにするためである。
type AgentConnector interface {
	// Address は接続ログに出す宛先である。空なら agent は設定されていない。
	Address() string
	// Connect は agent への接続を開く。署名の要求は認証や転送のあいだのいつ来るか
	// 分からないので、開いた後の読み書きに期限を付けない実装を渡す。
	Connect(ctx context.Context) (net.Conn, error)
}

// Auth は、認証に使えるものの全体である。
type Auth struct {
	// Stored は、その鍵について保存されているパスフレーズを返す。
	//
	// vault を見るのはここである。尋ねた結果は保存しない。保存は
	// Secrets 画面の仕事であり、接続の途中で暗黙に永続化しない。
	Stored func(path string) (string, bool)
	// Password は、その alias について保存されているアカウントパスワードを返す。
	//
	// Stored とは別の名前空間である。Stored は秘密鍵のロック解除に使い、
	// Password はリモートアカウントの認証に使う。
	// 取り違えれば、鍵を開くための秘密がそのままリモートへ送られる。
	Password func(target Target) (string, bool)
	// TOTP は、明示的なワンタイムコード質問にだけ現在のコードを返す。
	// provisioning secretの判定と時刻依存の生成はprovider側に閉じ込める。
	TOTP func(target Target, question string) (string, bool)
	// Agent は ssh-agent の宛先と開き方である。nil か宛先が空なら agent は使わない。
	Agent AgentConnector
	// ReadFile は鍵ファイルを読む。テストがフィクスチャを渡すためにある。
	// nil なら os.ReadFile。
	ReadFile func(path string) ([]byte, error)
	// Observe は、方式が実際に試された瞬間に呼ばれる。
	//
	// ssh.AuthMethod は暗号化された interface なので、外から包めない。
	// どの方式で通ったかを言えるのは、方式を組み立てるここだけである。
	Observe func(method string)
	// ObserveCredential は、保存済み資格情報を認証質問へ使えたかを、秘密値を
	// 渡さずに報告する。接続ログ専用であり、コードや質問への回答は含めない。
	ObserveCredential func(target Target, event CredentialEvent, echoed bool)
	// registerAgent is set on a per-handshake copy by methodsWithCleanup.
	// Agent signers keep their socket alive, so the dialer must close it once
	// authentication has completed or failed.
	registerAgent func(io.Closer)
	// trace は、この接続の接続ログである。authWithTrace が接続ごとの複製に
	// 置く。共有の Auth には無く、nil のまま呼んでも何も書かない。
	//
	// 秘密値は書かない。鍵は指紋で、パスフレーズとパスワードは出どころ
	// （保存済みか、入力か）だけで言う。
	trace *tracer
}

// CredentialEvent は、認証中の保存済み資格情報に関する安全な診断である。
type CredentialEvent string

const (
	CredentialTOTPUsed        CredentialEvent = "totp_used"
	CredentialTOTPUnavailable CredentialEvent = "totp_unavailable"
)

func (a Auth) methodsWithCleanup(target Target, prompt Prompter) ([]ssh.AuthMethod, func()) {
	var closers []io.Closer
	a.registerAgent = func(closer io.Closer) { closers = append(closers, closer) }
	return a.Methods(target, prompt), func() { closeAll(closers) }
}

func (a Auth) observe(method string) {
	if a.Observe != nil {
		a.Observe(method)
	}
}

func (a Auth) observeCredential(target Target, event CredentialEvent, echoed bool) {
	if a.ObserveCredential != nil {
		a.ObserveCredential(target, event, echoed)
	}
}

// Methods は、この接続で試す認証方式を、試す順に返す。
//
// 返すのは ssh.AuthMethod の並びであり、どれが通るかを決めるのはサーバーである。
// x/crypto/ssh は、サーバーが提示した方式と突き合わせて、この順に試す。
func (a Auth) Methods(target Target, prompt Prompter) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	// 保存された結果は、この接続を通して一度しか出さない。方式をまたいで
	// ひとつなのは、password と keyboard-interactive の両方を提示する
	// サーバーへ、同じ間違った結果を二度送らないためである。
	stored := a.newStoredCredentials(target)
	if isNonInteractive(prompt) {
		// provider が無いなら password 系を組み立てない。provider がある場合も、
		// ここでは vault を読まず、サーバーが実際に方式を提示した callback 内で
		// 初めて stored を呼ぶ。publickey で通る接続が password を取り出しては
		// ならない。
		if (a.Password == nil && a.TOTP == nil) || target.Alias == "" {
			prompt = nil
		}
	}
	order := target.Methods.Order()
	a.trace.say(connectionlog.Detailed, "認証方式の候補（試す順）：%s", strings.Join(order, ", "))
	for _, kind := range order {
		switch kind {
		case "publickey":
			if method, ok := a.publicKey(target, prompt); ok {
				methods = append(methods, method)
			} else {
				a.trace.say(connectionlog.Detailed, "publickeyは試しません：IdentityFileが無く、ssh-agentも使いません。")
			}
		case "keyboard-interactive", "password":
			if prompt == nil {
				a.trace.say(connectionlog.Detailed, "%sは試しません：非対話で、保存済みパスワードもTOTPもありません。", kind)
				continue
			}
			method := ssh.KeyboardInteractive(a.keyboard(target, prompt, stored))
			if kind == "password" {
				method = ssh.PasswordCallback(a.password(target, prompt, stored))
			}
			methods = append(methods, ssh.RetryableAuthMethod(method, maxPasswordAttempts))
		}
	}
	return methods
}

// password は、パスワード方式の結果を作る。
//
// 保存されているなら、それを出す。保管庫に置いてあるのに毎回尋ねるなら、
// 置く意味が無い。
func (a Auth) password(target Target, prompt Prompter, stored storedCredentials) func() (string, error) {
	return func() (string, error) {
		a.observe("password")
		rejected := stored.password.noteAsked()
		if password, found := stored.password.take(); found {
			a.trace.say(connectionlog.Detailed, "保存済みパスワードを送ります。")
			return password, nil
		}
		prefix := ""
		if rejected {
			prefix = "Saved password was rejected. "
			a.trace.say(connectionlog.Detailed, "保存済みパスワードが拒否されました。パスワードの入力を求めます。")
		} else {
			a.trace.say(connectionlog.Detailed, "パスワードの入力を求めます。")
		}
		answer, err := prompt.Secret(prefix + "Password for " + authenticationTarget(target) + ": ")
		return answer, stored.explainUnanswered(err)
	}
}

// keyboard は、keyboard-interactive の結果を作る。
//
// 保存されたパスワードで返すのは、問いがひとつで、画面に出さないときだけ
// である。それがパスワードを聞かれている形であり、普通の Linux はパスワードを
// この方式で聞いてくる。問いが複数あるもの（2FA）や、結果を画面に出す問いに
// パスワードを差し出す意味は無く、差し出せばそれは間違った結果になる。
func (a Auth) keyboard(target Target, prompt Prompter, stored storedCredentials) ssh.KeyboardInteractiveChallenge {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		a.observe("keyboard-interactive")
		a.traceChallenge(name, questions, echos)
		round := newKeyboardRound(questions, echos)
		passwordRejected := round.asksPassword() && stored.password.noteAsked()
		totpRejected := round.asksAnyTOTP() && stored.totp.noteAsked()
		for index, question := range questions {
			if !round.asksTOTP[index] || stored.totp.wasOffered() {
				continue
			}
			if code, found := stored.totp.take(question); found {
				round.answer(index, code)
				a.observeCredential(target, CredentialTOTPUsed, round.echoed(index))
				continue
			}
			a.observeCredential(target, CredentialTOTPUnavailable, round.echoed(index))
		}
		if totpRejected && isNonInteractive(prompt) {
			a.trace.say(connectionlog.Detailed, "保存済みTOTPが拒否されました。")
		} else if totpRejected {
			a.trace.say(connectionlog.Detailed, "保存済みTOTPが拒否されました。認証コードの入力を求めます。")
		}
		if index := round.storedPasswordQuestion(); index >= 0 {
			if password, found := stored.password.take(); found {
				round.answer(index, password)
			}
		}
		a.trace.say(connectionlog.Detailed, "keyboard-interactive：保存済みの認証情報で%d件、入力で%d件に答えます。",
			countAnswered(round.answered), len(round.answered)-countAnswered(round.answered))
		if allAnswered(round.answered) {
			return round.answers, nil
		}
		context := "Authentication for " + authenticationTarget(target)
		if totpRejected {
			context = "Saved verification code was rejected.\r\n" + context
		}
		if passwordRejected {
			context = "Saved password was rejected.\r\n" + context
		}
		challenge := keyboardChallenge{
			context: context, name: name, instruction: instruction, questions: questions, echos: echos,
		}
		answers, err := answerKeyboardChallenge(prompt, challenge, challengeAnswers{values: round.answers, answered: round.answered})
		return answers, stored.explainUnanswered(err)
	}
}

// keyboardRound は、keyboard-interactive の1回の問いと、保存済みの値で答えた結果である。
type keyboardRound struct {
	questions []string
	echos     []bool
	answers   []string
	answered  []bool
	// asksTOTP は、質問がTOTPを尋ねているか。保存済みTOTPで答えたかとは別に持つ。
	// 拒否されたあとや保存が無いときも、TOTPの質問であることは変わらない。
	asksTOTP []bool
}

func newKeyboardRound(questions []string, echos []bool) keyboardRound {
	round := keyboardRound{
		questions: questions,
		echos:     echos,
		answers:   make([]string, len(questions)),
		answered:  make([]bool, len(questions)),
		asksTOTP:  make([]bool, len(questions)),
	}
	for index, question := range questions {
		round.asksTOTP[index] = totp.MatchesPrompt(question)
	}
	return round
}

func (round keyboardRound) answer(index int, value string) {
	round.answers[index] = value
	round.answered[index] = true
}

// asksPassword は、この問いがアカウントのパスワードを尋ねているか。
// 保存済みパスワードを送ったあとなら、送ったものは拒否されている。
func (round keyboardRound) asksPassword() bool {
	if round.hasOneHiddenNonTOTPQuestion() {
		return true
	}
	for index, question := range round.questions {
		if round.hidden(index) && isPasswordQuestion(question) {
			return true
		}
	}
	return false
}

func (round keyboardRound) asksAnyTOTP() bool {
	for _, asks := range round.asksTOTP {
		if asks {
			return true
		}
	}
	return false
}

func (round keyboardRound) echoed(index int) bool {
	return index < len(round.echos) && round.echos[index]
}

func (round keyboardRound) hidden(index int) bool {
	return index < len(round.echos) && !round.echos[index]
}

// storedPasswordQuestion は、保存済みのアカウントパスワードで答えてよい質問の
// 位置を返す。無ければ -1 を返す。
//
// TOTPの質問には、保存済みTOTPが拒否されたあとでも、無かったときでも、
// パスワードを答えない。答えればアカウントのパスワードが別の用途の欄へ送られ、
// 正しい保存値まで拒否されたように見える。
func (round keyboardRound) storedPasswordQuestion() int {
	// A combined password+OTP challenge is common. Only after an explicit OTP
	// question has been recognised do we release a saved password to the one
	// remaining, strictly named Password question. This keeps the previous
	// multi-question refusal for arbitrary challenges.
	if anyAnswered(round.answered) {
		passwordIndex := -1
		for index, question := range round.questions {
			if round.answered[index] || !round.hidden(index) || !isPasswordQuestion(question) {
				continue
			}
			if passwordIndex != -1 {
				return -1
			}
			passwordIndex = index
		}
		return passwordIndex
	}
	if round.hasOneHiddenNonTOTPQuestion() {
		return 0
	}
	return -1
}

// hasOneHiddenNonTOTPQuestion は、問いがひとつで、画面に出さず、TOTPでもないかを返す。
// 普通の Linux が keyboard-interactive でパスワードを聞く形である。
func (round keyboardRound) hasOneHiddenNonTOTPQuestion() bool {
	return len(round.questions) == 1 && len(round.echos) == 1 && round.hidden(0) && !round.asksTOTP[0]
}

// traceChallenge は、サーバーが出した keyboard-interactive の問いを接続ログに書く。
//
// 問いの文はサーバーが書いたものなので、端末へ出す前に制御文字を落とす。
// 保存済みの資格情報で答えた問いはユーザーの画面に出ないため、何を聞かれて
// いたかを知る手段はこの行だけである。
func (a Auth) traceChallenge(name string, questions []string, echos []bool) {
	if !a.trace.enabled(connectionlog.Full) {
		return
	}
	if name != "" {
		a.trace.say(connectionlog.Full, "keyboard-interactiveの名前：%s", terminal.DisplayText(name, maxChallengeTextRunes))
	}
	for index, question := range questions {
		echoed := index < len(echos) && echos[index]
		a.trace.say(connectionlog.Full, "keyboard-interactiveのプロンプト%d/%d：%s（入力表示：%s）",
			index+1, len(questions), terminal.DisplayText(question, maxChallengeTextRunes),
			map[bool]string{true: "あり", false: "なし"}[echoed])
	}
}

func countAnswered(answered []bool) int {
	count := 0
	for _, ok := range answered {
		if ok {
			count++
		}
	}
	return count
}

func allAnswered(answered []bool) bool {
	for _, ok := range answered {
		if !ok {
			return false
		}
	}
	return true
}

func anyAnswered(answered []bool) bool {
	for _, ok := range answered {
		if ok {
			return true
		}
	}
	return false
}

func isPasswordQuestion(question string) bool {
	question = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(question), ":"))
	return strings.EqualFold(question, "password") || question == "パスワード"
}

// publicKey は、鍵を必要になった時点で読む認証方式を組み立てる。
//
// 遅延させるのは、パスフレーズを尋ねるのが認証の最中だからである。接続を
// 始める前にすべての鍵を復号すると、公開鍵認証を提示すらしないサーバーに対して
// パスフレーズを尋ねることになる。
func (a Auth) publicKey(target Target, prompt Prompter) (ssh.AuthMethod, bool) {
	if len(target.Identities) == 0 && (target.IdentitiesOnly || !a.agentConfigured()) {
		// 鍵がひとつも無い。OpenSSH の既定の探索順（~/.ssh/id_ed25519 など）は
		// 持たない。バージョンとビルドで変わる表であり、internal/effective が
		// IdentityFile の既定を持たないのと同じ理由である。
		return nil, false
	}
	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		a.observe("publickey")
		return a.Signers(target, prompt)
	}), true
}

// Signers は、この接続で試す鍵を集める。
//
// 公開されているのは、鍵がひとつも無いことが独立した失敗であり、それを
// 検査から指定できる必要があるからである。
func (a Auth) Signers(target Target, prompt Prompter) ([]ssh.Signer, error) {
	var signers []ssh.Signer
	var failures []string

	for _, path := range target.Identities {
		signer, unlockedBy, err := a.signerFor(path, prompt)
		if err != nil {
			// 他の鍵で通れば、この失敗は誰にも報告されない。書けない鍵が
			// 混ざっていることに気づけるのは接続ログだけである。
			a.trace.say(connectionlog.Detailed, "鍵%sは使えません：%v", path, err)
			failures = append(failures, path+": "+err.Error())
			continue
		}
		a.trace.say(connectionlog.Detailed, "鍵%s：%s（%s）", path, describeKey(signer.PublicKey()), unlockedBy)
		signers = append(signers, signer)
	}

	// IdentitiesOnly yes は、設定に書かれた鍵だけを使うという指定である。
	if !target.IdentitiesOnly && a.agentConfigured() {
		agentSigners, err := a.agentSigners()
		if err != nil {
			a.trace.say(connectionlog.Detailed, "ssh-agentの鍵は使えません：%v", err)
			failures = append(failures, "agent: "+err.Error())
		}
		signers = append(signers, agentSigners...)
	} else if target.IdentitiesOnly && a.agentConfigured() {
		a.trace.say(connectionlog.Detailed, "IdentitiesOnly yesのためssh-agentの鍵は使いません。")
	}

	if len(signers) == 0 {
		if len(failures) == 0 {
			return nil, ErrNoIdentity
		}
		return nil, fmt.Errorf("%w (%s)", ErrNoIdentity, strings.Join(failures, "; "))
	}
	a.trace.say(connectionlog.Detailed, "公開鍵認証で試す鍵：%d件", len(signers))
	return signers, nil
}

// 鍵をどうやって使える形にしたか。接続ログが鍵ごとに言う。
const (
	unlockedWithoutPassphrase = "パスフレーズ無し"
	unlockedWithStored        = "保存済みパスフレーズで復号"
	unlockedWithTyped         = "入力したパスフレーズで復号"
)

// signerFor は、鍵ファイルを読んで署名できる形にし、どう復号したかも返す。
func (a Auth) signerFor(path string, prompt Prompter) (ssh.Signer, string, error) {
	contents, err := a.read(path)
	if err != nil {
		return nil, "", err
	}

	private, err := keys.DecodePrivateKey(contents, nil)
	if err == nil {
		signer, err := ssh.NewSignerFromKey(private)
		return signer, unlockedWithoutPassphrase, err
	}
	if !errors.Is(err, keys.ErrPassphraseRequired) {
		return nil, "", err
	}

	// 保存されているパスフレーズを先に試す。ユーザーに尋ねる前に、結果を既に
	// 持っているかを見る。
	if a.Stored != nil {
		if passphrase, found := a.Stored(path); found {
			private, err := keys.DecodePrivateKey(contents, []byte(passphrase))
			if err == nil {
				signer, err := ssh.NewSignerFromKey(private)
				return signer, unlockedWithStored, err
			}
			if !errors.Is(err, keys.ErrWrongPassphrase) {
				return nil, "", err
			}
			a.trace.say(connectionlog.Detailed, "鍵%s：保存済みパスフレーズが合いません。", path)
		}
	}
	if prompt == nil {
		return nil, "", keys.ErrPassphraseRequired
	}

	for attempt := 0; attempt < maxPassphraseAttempts; attempt++ {
		passphrase, err := prompt.Secret("Enter passphrase for key '" + path + "': ")
		if err != nil {
			return nil, "", err
		}
		private, err := keys.DecodePrivateKey(contents, []byte(passphrase))
		if err == nil {
			signer, err := ssh.NewSignerFromKey(private)
			return signer, unlockedWithTyped, err
		}
		if !errors.Is(err, keys.ErrWrongPassphrase) {
			return nil, "", err
		}
	}
	return nil, "", keys.ErrWrongPassphrase
}

// agentConfigured は、agent の宛先があるかを報告する。届くかどうかは開くまで分からず、
// 開けなければ公開鍵認証の失敗として扱って次の方式へ進む。
func (a Auth) agentConfigured() bool {
	return a.Agent != nil && a.Agent.Address() != ""
}

func (a Auth) agentSigners() ([]ssh.Signer, error) {
	conn, err := a.Agent.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	if a.registerAgent != nil {
		a.registerAgent(conn)
	}
	// Signer は認証中にこの接続を使う。methodsWithCleanupを使う接続経路は
	// handshake終了時に明示的に閉じる。
	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		return nil, err
	}
	a.trace.say(connectionlog.Detailed, "ssh-agentの鍵：%d件（%s）", len(signers), a.Agent.Address())
	for _, signer := range signers {
		a.trace.say(connectionlog.Full, "ssh-agentの鍵：%s", describeKey(signer.PublicKey()))
	}
	return signers, nil
}

func (a Auth) read(path string) ([]byte, error) {
	if a.ReadFile != nil {
		return a.ReadFile(path)
	}
	return os.ReadFile(path)
}

// maxChallengeTextRunes bounds each keyboard-interactive name, instruction and
// question before it is written to the terminal.
const maxChallengeTextRunes = 1024

// keyboardChallenge は、サーバーが出した keyboard-interactive の問いひとそろいと、
// その前に置くこちらの説明（context）である。
type keyboardChallenge struct {
	context, name, instruction string
	questions                  []string
	echos                      []bool
}

// challengeAnswers は、問いごとの答えと、保存済みの資格情報ですでに答えたかである。
type challengeAnswers struct {
	values   []string
	answered []bool
}

// answerKeyboardChallenge は、まだ答えていない問いを利用者に尋ね、答えを返す。
func answerKeyboardChallenge(prompt Prompter, challenge keyboardChallenge, answers challengeAnswers) ([]string, error) {
	// name と instruction は、サーバーがユーザーへ向けて書いた文である。捨てると
	// 「何を応答すればよいか」がそのユーザーに届かない。最初の未回答の問いの前に置く。
	// ただし端末へそのまま書く文なので、OpenSSH の vis() と同じく制御文字は落とす。
	preamble := strings.TrimSpace(strings.TrimSpace(challenge.context) + "\r\n" +
		terminal.DisplayText(challenge.name, maxChallengeTextRunes) + "\r\n" +
		terminal.DisplayText(challenge.instruction, maxChallengeTextRunes))
	preambleShown := false
	for index, question := range challenge.questions {
		if index < len(answers.answered) && answers.answered[index] {
			continue
		}
		ask := prompt.Secret
		if index < len(challenge.echos) && challenge.echos[index] {
			ask = prompt.Line
		}
		question = terminal.DisplayText(question, maxChallengeTextRunes)
		if !preambleShown && preamble != "" {
			question = preamble + "\r\n" + question
			preambleShown = true
		}
		answer, err := ask(question)
		if err != nil {
			return nil, err
		}
		answers.values[index] = answer
	}
	return answers.values, nil
}

func authenticationTarget(target Target) string {
	where := target.User + "@" + target.Address()
	if target.Alias != "" && target.Alias != target.HostName {
		where += " (" + target.Alias + ")"
	}
	return where
}
