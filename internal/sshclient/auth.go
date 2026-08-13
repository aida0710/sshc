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

// Auth は、認証に使えるものの全体である。
type Auth struct {
	// Stored は、その鍵について保存されているパスフレーズを返す。
	//
	// vault を見るのはここである。**尋ねた答えは保存しない**——保存は
	// Secrets 画面の仕事であり、接続の途中で黙って永続化しない。
	Stored func(path string) (string, bool)
	// AgentSocket は SSH_AUTH_SOCK。空なら agent は使わない。
	AgentSocket string
	// ReadFile は鍵ファイルを読む。テストがフィクスチャを渡すためにある。
	// nil なら os.ReadFile。
	ReadFile func(path string) ([]byte, error)
}

// Methods は、この接続で試す認証方式を、試す順に返す。
//
// 返すのは ssh.AuthMethod の並びであり、どれが通るかを決めるのはサーバーである。
// x/crypto/ssh は、サーバーが提示した方式と突き合わせて、この順に試す。
func (a Auth) Methods(target Target, prompt Prompter) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	for _, kind := range target.Methods.Order() {
		switch kind {
		case "publickey":
			if method, ok := a.publicKey(target, prompt); ok {
				methods = append(methods, method)
			}
		case "keyboard-interactive":
			if prompt != nil {
				methods = append(methods, ssh.KeyboardInteractive(keyboardChallenge(prompt)))
			}
		case "password":
			if prompt != nil {
				methods = append(methods, ssh.PasswordCallback(func() (string, error) {
					return prompt.Secret("Password: ")
				}))
			}
		}
	}
	return methods
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
