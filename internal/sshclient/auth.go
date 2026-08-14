package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/keys"
)

// ErrNoIdentity は、公開鍵認証に使える鍵がひとつも無いことを報告する。
var ErrNoIdentity = errors.New("no identity file and no agent key is available")

// maxPassphraseAttempts は、ひとつの鍵についてパスフレーズを尋ねる回数である。
//
// OpenSSH と同じ 3 回。上限が無いと、間違え続ける人がこの接続を保持し続ける。
const maxPassphraseAttempts = 3

// maxPasswordAttempts は、ひとつの方式を試し直す回数である。
//
// OpenSSH の NumberOfPasswordPrompts と同じ 3 回。**保存された答えが古いときに
// 繋ぐ道が残っている**のはこの回数のおかげであり、一度で諦めると、その alias は
// 保管庫を直すまで開けなくなる。
const maxPasswordAttempts = 3

// Auth は、認証に使えるものの全体である。
type Auth struct {
	// Stored は、その鍵について保存されているパスフレーズを返す。
	//
	// vault を見るのはここである。**尋ねた答えは保存しない**——保存は
	// Secrets 画面の仕事であり、接続の途中で黙って永続化しない。
	Stored func(path string) (string, bool)
	// Password は、その alias について保存されているアカウントパスワードを返す。
	//
	// **Stored とは別の名前空間である。** あちらはローカルの秘密鍵を開くための
	// 秘密で、こちらはリモートのアカウントへログインするための秘密である。
	// 取り違えれば、鍵を開くための秘密がそのままリモートへ送られる。
	Password func(alias string) (string, bool)
	// AgentSocket は SSH_AUTH_SOCK。空なら agent は使わない。
	AgentSocket string
	// ReadFile は鍵ファイルを読む。テストがフィクスチャを渡すためにある。
	// nil なら os.ReadFile。
	ReadFile func(path string) ([]byte, error)
	// Observe は、方式が実際に試された瞬間に呼ばれる。
	//
	// **ssh.AuthMethod は封じられた interface なので、外から包めない。**
	// どの方式で通ったかを言えるのは、方式を組み立てるここだけである。
	Observe func(method string)
}

func (a Auth) observe(method string) {
	if a.Observe != nil {
		a.Observe(method)
	}
}

// Methods は、この接続で試す認証方式を、試す順に返す。
//
// 返すのは ssh.AuthMethod の並びであり、どれが通るかを決めるのはサーバーである。
// x/crypto/ssh は、サーバーが提示した方式と突き合わせて、この順に試す。
func (a Auth) Methods(target Target, prompt Prompter) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	// 保存された答えは、この接続を通して一度しか出さない。方式をまたいで
	// ひとつなのは、password と keyboard-interactive の両方を提示する
	// サーバーへ、同じ間違った答えを二度送らないためである。
	stored := a.storedPassword(target)
	for _, kind := range target.Methods.Order() {
		switch kind {
		case "publickey":
			if method, ok := a.publicKey(target, prompt); ok {
				methods = append(methods, method)
			}
		case "keyboard-interactive":
			if prompt != nil {
				methods = append(methods, ssh.RetryableAuthMethod(
					ssh.KeyboardInteractive(a.keyboard(prompt, stored)), maxPasswordAttempts))
			}
		case "password":
			if prompt != nil {
				methods = append(methods, ssh.RetryableAuthMethod(
					ssh.PasswordCallback(a.password(prompt, stored)), maxPasswordAttempts))
			}
		}
	}
	return methods
}

// storedPassword は、保存されたパスワードを一度だけ差し出す関数を返す。
//
// **一度だけなのは、保存された答えが古いことがあるからである。** 断られても
// 同じものを出し続ければ、繋ぐ道が残らない。二度目からは人に尋ねる。
func (a Auth) storedPassword(target Target) func() (string, bool) {
	if a.Password == nil || target.Alias == "" {
		return func() (string, bool) { return "", false }
	}
	offered := false
	return func() (string, bool) {
		if offered {
			return "", false
		}
		offered = true
		return a.Password(target.Alias)
	}
}

// password は、パスワード方式の答えを作る。
//
// **保存されているなら、それを出す。** 保管庫に置いてあるのに毎回尋ねるなら、
// 置く意味が無い。
func (a Auth) password(prompt Prompter, stored func() (string, bool)) func() (string, error) {
	return func() (string, error) {
		a.observe("password")
		if password, found := stored(); found {
			return password, nil
		}
		return prompt.Secret("Password: ")
	}
}

// keyboard は、keyboard-interactive の答えを作る。
//
// **保存されたパスワードで答えるのは、問いがひとつで、画面に出さないときだけ
// である。** それがパスワードを聞かれている形であり、普通の Linux はパスワードを
// この方式で聞いてくる。問いが複数あるもの（2FA）や、答えを画面に出す問いに
// パスワードを差し出す意味は無く、差し出せばそれは間違った答えになる。
func (a Auth) keyboard(prompt Prompter, stored func() (string, bool)) ssh.KeyboardInteractiveChallenge {
	ask := keyboardChallenge(prompt)
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		a.observe("keyboard-interactive")
		if len(questions) == 1 && len(echos) == 1 && !echos[0] {
			if password, found := stored(); found {
				return []string{password}, nil
			}
		}
		return ask(name, instruction, questions, echos)
	}
}

// publicKey は、鍵を必要になった時点で読む認証方式を組み立てる。
//
// 遅延させるのは、パスフレーズを尋ねるのが認証の最中だからである。接続を
// 始める前にすべての鍵を復号すると、公開鍵認証を提示すらしないサーバーに対して
// パスフレーズを尋ねることになる。
func (a Auth) publicKey(target Target, prompt Prompter) (ssh.AuthMethod, bool) {
	if len(target.Identities) == 0 && (target.IdentitiesOnly || a.AgentSocket == "") {
		// 鍵がひとつも無い。OpenSSH の既定の探索順（~/.ssh/id_ed25519 など）は
		// 持たない——B1 が既定値表を持たないと決めた理由がそのまま当てはまる。
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
// 検査から名指しできる必要があるからである。
func (a Auth) Signers(target Target, prompt Prompter) ([]ssh.Signer, error) {
	var signers []ssh.Signer
	var failures []string

	for _, path := range target.Identities {
		signer, err := a.signerFor(path, prompt)
		if err != nil {
			failures = append(failures, path+": "+err.Error())
			continue
		}
		signers = append(signers, signer)
	}

	// IdentitiesOnly yes は、設定に書かれた鍵だけを使うという指定である。
	if !target.IdentitiesOnly && a.AgentSocket != "" {
		agentSigners, err := a.agentSigners()
		if err != nil {
			failures = append(failures, "agent: "+err.Error())
		}
		signers = append(signers, agentSigners...)
	}

	if len(signers) == 0 {
		if len(failures) == 0 {
			return nil, ErrNoIdentity
		}
		return nil, fmt.Errorf("%w (%s)", ErrNoIdentity, strings.Join(failures, "; "))
	}
	return signers, nil
}

func (a Auth) signerFor(path string, prompt Prompter) (ssh.Signer, error) {
	contents, err := a.read(path)
	if err != nil {
		return nil, err
	}

	private, err := keys.DecodePrivateKey(contents, nil)
	if err == nil {
		return ssh.NewSignerFromKey(private)
	}
	if !errors.Is(err, keys.ErrPassphraseRequired) {
		return nil, err
	}

	// 保存されているパスフレーズを先に試す。人に尋ねる前に、答えを既に
	// 持っているかを見る。
	if a.Stored != nil {
		if passphrase, found := a.Stored(path); found {
			private, err := keys.DecodePrivateKey(contents, []byte(passphrase))
			if err == nil {
				return ssh.NewSignerFromKey(private)
			}
			if !errors.Is(err, keys.ErrWrongPassphrase) {
				return nil, err
			}
		}
	}
	if prompt == nil {
		return nil, keys.ErrPassphraseRequired
	}

	for attempt := 0; attempt < maxPassphraseAttempts; attempt++ {
		passphrase, err := prompt.Secret("Enter passphrase for key '" + path + "': ")
		if err != nil {
			return nil, err
		}
		private, err := keys.DecodePrivateKey(contents, []byte(passphrase))
		if err == nil {
			return ssh.NewSignerFromKey(private)
		}
		if !errors.Is(err, keys.ErrWrongPassphrase) {
			return nil, err
		}
	}
	return nil, keys.ErrWrongPassphrase
}

func (a Auth) agentSigners() ([]ssh.Signer, error) {
	conn, err := net.Dial("unix", a.AgentSocket)
	if err != nil {
		return nil, err
	}
	// 接続はここで閉じない。返した Signer が署名のたびに使う。
	return agent.NewClient(conn).Signers()
}

func (a Auth) read(path string) ([]byte, error) {
	if a.ReadFile != nil {
		return a.ReadFile(path)
	}
	return os.ReadFile(path)
}

// keyboardChallenge は、サーバーの質問をそのまま人へ渡す。
//
// 質問文を作るのはサーバーである。こちらが言い換えると、2FA の指示が
// 変わってしまう。
func keyboardChallenge(prompt Prompter) ssh.KeyboardInteractiveChallenge {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, 0, len(questions))
		// name と instruction は、サーバーが人へ向けて書いた文である。捨てると
		// 「何を答えればよいか」がその人に届かない。最初の問いの前に置く。
		preamble := strings.TrimSpace(strings.TrimSpace(name) + "\r\n" + strings.TrimSpace(instruction))
		for index, question := range questions {
			ask := prompt.Secret
			if index < len(echos) && echos[index] {
				ask = prompt.Line
			}
			if index == 0 && preamble != "" {
				question = preamble + "\r\n" + question
			}
			answer, err := ask(question)
			if err != nil {
				return nil, err
			}
			answers = append(answers, answer)
		}
		return answers, nil
	}
}
