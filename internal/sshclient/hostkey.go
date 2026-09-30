package sshclient

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/knownhosts"
)

// ホスト鍵を受け入れなかった理由。
var (
	// ErrHostKeyChanged は、known_hosts にある鍵と違う鍵を提示したホストを断る。
	//
	// ここだけはユーザーに判断させない。known_hosts にあって鍵が違うのは中間者攻撃の
	// 形そのものであり、「続けますか」と尋ねること自体が攻撃の成立条件になる。
	ErrHostKeyChanged = errors.New("the host key does not match the one in known_hosts")
	// ErrHostKeyUnknown は、未知のホストを受け入れなかったことを報告する。
	ErrHostKeyUnknown = errors.New("this host is not in known_hosts")
	// ErrHostKeyRevoked は、@revoked と印の付いた鍵を断る。
	ErrHostKeyRevoked = errors.New("this host key is marked revoked in known_hosts")
	// ErrHostKeyChangedDuringRekey は、接続中の鍵の再交換（rekey）で、最初の鍵交換と
	// 違うホスト鍵を提示したホストを断る。OpenSSH も rekey では最初のホスト鍵と
	// 同じ鍵であることを求める。
	ErrHostKeyChangedDuringRekey = errors.New("the server host key changed during rekey")
)

// KnownHostsSymlinkError は、受け入れたホスト鍵を保存する known_hosts のパスに
// シンボリックリンクがあり、保存できなかったことを報告する。
//
// sshc は ~/.ssh の中の実ファイルにだけ書き、リンクはたどらない。保存しないまま
// 接続を続けると、accept-new では未知のホストを毎回確認なしで受け入れることになる
// ので、接続は失敗にする。OpenSSH はリンク先へ書くので、ssh で一度接続すれば登録できる。
type KnownHostsSymlinkError struct {
	// Path は、保存しようとした UserKnownHostsFile の最初のファイルである。
	Path string
	Err  error
}

func (failure *KnownHostsSymlinkError) Error() string {
	return "cannot save the host key to " + failure.Path +
		": the path contains a symbolic link, and sshc does not write through links; " +
		"connect once with ssh to save the key, then connect with sshc again"
}

func (failure *KnownHostsSymlinkError) Unwrap() error { return failure.Err }

// HostKeys は、known_hosts との突き合わせである。
//
// 読み書きを関数として受け取り、UI と接続処理が同じ known_hosts を使用できるようにする。
// どのファイルを読み、どこへ書くかは Target.KnownHosts が決める。
type HostKeys struct {
	// Read は known_hosts のファイルひとつの中身を返す。無いファイルは空として返す。
	// nil なら、既知のホストは一つも無い。
	Read func(path string) ([]byte, error)
	// Add は受け入れた鍵を path の known_hosts へ書く。nil なら覚えない。接続は
	// できるが、次も尋ねる。knownhosts.ErrNotWritable は、書かない場所を指していた
	// ことを表し、接続は止めない。knownhosts.ErrSymlinkPath は接続を失敗にする
	// （KnownHostsSymlinkError）。
	Add func(path string, candidate knownhosts.Candidate) error
}

// hostKeyLookup は、ひとつの接続（ホップ）の known_hosts の照合である。
//
// 名乗るホスト鍵アルゴリズムの選択（algorithms）と鍵の照合（callback）は、同じ
// known_hosts の行を使う。ファイルは一度だけ読んで解析する。組織が配る
// GlobalKnownHostsFile は数十 MiB になり、接続のたびに二度読むと接続が遅くなる。
//
// x/crypto は、接続中の鍵の再交換（rekey）のたびにも callback を呼ぶ。rekey では
// known_hosts と照合し直さず、最初の鍵交換で受け入れた鍵と同じかだけを見る。
// 読んだ行には、その接続で確認して受け入れた鍵が入っていない。照合し直すと、
// rekey のたびにセッションの途中で同じ問いを出し、利用者の打鍵を答えとして読む。
type hostKeyLookup struct {
	keys   HostKeys
	target Target

	load    sync.Once
	entries []hostEntry
	err     error

	// mutex は firstKey を守る。
	mutex sync.Mutex
	// firstKey は、最初の鍵交換で受け入れたホスト鍵（SSH の wire 形式）である。
	// まだ受け入れていなければ nil。
	firstKey []byte
}

func (h HostKeys) lookup(target Target) *hostKeyLookup {
	return &hostKeyLookup{keys: h, target: target}
}

// hostEntries は、この接続先についての known_hosts の行を、最初の呼び出しで一度だけ集める。
func (lookup *hostKeyLookup) hostEntries() ([]hostEntry, error) {
	lookup.load.Do(func() { lookup.entries, lookup.err = lookup.keys.entriesFor(lookup.target) })
	return lookup.entries, lookup.err
}

// callback は、この接続のためのホスト鍵検証を返し、検証の経過を接続ログにも書く。
//
// 問いを出す先を引数で受け取るのは、それが接続ごとに違うからである。尋ねる
// のは、その接続を開いたターミナルでなければならない。別のターミナルに出た問いは、
// 誰も判定できないまま接続を止める。
//
// 鍵の指紋と照合の結果を言うのは、「一致しない鍵」と断られたユーザーが、
// どの鍵が来て known_hosts のどれと比べたのかを知る手段が他に無いからである。
func (lookup *hostKeyLookup) callback(prompt Prompter, trace *tracer) ssh.HostKeyCallback {
	target := lookup.target
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		trace.say(connectionlog.Detailed, "サーバーのホスト鍵：%s", describeKey(key))
		verdict, err := lookup.verify(key, prompt)
		for _, conflicting := range verdict.conflicting {
			trace.say(connectionlog.Detailed, "%sの%d行目には別の鍵があります：%s", conflicting.at.path, conflicting.at.number, conflicting.key)
		}
		if err != nil {
			trace.say(connectionlog.Detailed, "ホスト鍵を受け入れませんでした：%v", err)
			return err
		}
		switch verdict.outcome {
		case hostKeyKnown:
			trace.say(connectionlog.Detailed, "ホスト鍵は%sの%d行目と一致しました。", verdict.matched.path, verdict.matched.number)
		case hostKeyUnchanged:
			trace.say(connectionlog.Detailed, "鍵の再交換：ホスト鍵は最初の鍵交換と同じです。")
		case hostKeyTrusted:
			trace.say(connectionlog.Detailed, "known_hostsに無いホストを、StrictHostKeyChecking %sに従って受け入れました。", target.Strict)
		case hostKeyConfirmed:
			trace.say(connectionlog.Detailed, "known_hostsに無いホストを、確認のうえ受け入れました。")
		}
		acceptedUnknownHost := verdict.outcome == hostKeyTrusted || verdict.outcome == hostKeyConfirmed
		if acceptedUnknownHost && verdict.rememberedIn == "" {
			trace.say(connectionlog.Detailed, "この鍵はknown_hostsに書きません。次の接続でも同じ確認になります。")
		}
		return nil
	}
}

// hostKeyVerdict は、提示された鍵をどう扱ったかである。接続ログが言うためにある。
//
// 断った場合にも conflicting は埋まる。「一致しない鍵」と言われたユーザーが
// 見たいのは、どのファイルのどの行のどの鍵と比べたかである。
type hostKeyVerdict struct {
	outcome hostKeyOutcome
	// matched は、一致した known_hosts の行である。outcome が hostKeyKnown の
	// ときだけ意味を持つ。
	matched knownHostsLocation
	// conflicting は、同じホストについて別の鍵を書いている known_hosts の行である。
	conflicting []conflictingHostKey
	// rememberedIn は、受け入れた鍵を書いたファイルである。書かなかったら空。
	rememberedIn string
}

// knownHostsLocation は、known_hosts のファイルと、その中の行番号である。
type knownHostsLocation struct {
	path   string
	number int
}

type conflictingHostKey struct {
	at knownHostsLocation
	// key は種類と指紋である。鍵そのものは書かない。長すぎて突き合わせに向かない。
	key string
}

// hostKeyOutcome は、提示された鍵をなぜ受け入れたかである。
type hostKeyOutcome int

const (
	// hostKeyKnown は、known_hosts の行と一致した。
	hostKeyKnown hostKeyOutcome = iota + 1
	// hostKeyTrusted は、StrictHostKeyChecking が尋ねずに受け入れる設定だった。
	hostKeyTrusted
	// hostKeyConfirmed は、ユーザーが問いに yes と答えた。
	hostKeyConfirmed
	// hostKeyUnchanged は、鍵の再交換（rekey）で、最初の鍵交換で受け入れた鍵と
	// 同じ鍵が来た。
	hostKeyUnchanged
)

// defaultHostKeyAlgorithms は、他に手がかりが無いときに名乗る順である。
//
// 順番は OpenSSH の既定に合わせてある。x/crypto の既定表は ECDSA と RSA を
// Ed25519 より前に置くが、それに従うと、初めて繋ぐホストについて覚える鍵の種類が
// `ssh` の覚えるものと変わる。同じ known_hosts を二つのクライアントが書くのだから、
// 順番は揃っている方がよい。
//
// 証明書のアルゴリズムは入れない。このクライアントは証明書を読まないので、
// 名乗れば、受け取っても突き合わせられないものを相手に選ばせることになる。
var defaultHostKeyAlgorithms = []string{
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
}

// algorithms は、この接続で名乗るホスト鍵アルゴリズムを優先順に返す。
//
// known_hosts に持っている種類を先に置く。これが無いと順番を決めるのは
// x/crypto の既定表になり、そこでは RSA と ECDSA が Ed25519 より前にある。
// 三種類の鍵を持つ普通の Ubuntu が相手だと RSA が選ばれ、known_hosts にある
// のが ed25519 の 1 行だけなら、正しいホストの正しい鍵が「一致しない鍵」として
// 現れる。実際そうなっていた。変わったのは相手ではなく、こちらの選び方で
// ある。OpenSSH が同じ場面で ed25519 を選ぶのは、すでに持っている種類を
// 先に置いているからで、ここがしているのはそれと同じことである。
//
// 設定に HostKeyAlgorithms が書かれていれば、それが順序である。OpenSSH は
// その指定があるとき known_hosts による並べ替えを行わない。ユーザーが決めた順を、
// こちらの都合で作り変えない。
//
// 知らないホストでは既定の順を返す。持っていない鍵について主張することは無いが、
// 何も渡さなければ x/crypto の順になり、それは `ssh` の順ではない。
func (lookup *hostKeyLookup) algorithms() []string {
	if len(lookup.target.HostKeyAlgorithms) > 0 {
		return lookup.target.HostKeyAlgorithms
	}
	// 読めないことをここで報告する必要はない。同じ読み取りの誤りは検証でも
	// 返り、そこが接続を止める。
	entries, _ := lookup.hostEntries()
	var algorithms []string
	seen := map[string]bool{}
	for _, found := range entries {
		// 印の付いた行は、この接続で受け入れる鍵ではない。@revoked は拒む鍵で
		// あり、@cert-authority は鍵ではなく署名者である。
		if found.entry.Marker != "" {
			continue
		}
		for _, algorithm := range signatureAlgorithms(found.entry.KeyType) {
			if seen[algorithm] {
				continue
			}
			seen[algorithm] = true
			algorithms = append(algorithms, algorithm)
		}
	}
	if len(algorithms) == 0 {
		return defaultHostKeyAlgorithms
	}
	return algorithms
}

// Knows は、照合に使う known_hosts のどれかが、このホストの鍵をすでに持っているかを
// 報告する。@revoked と @cert-authority の行は鍵として数えない。
//
// 保存したパスワードで繋ぐ非対話の経路は、未知のホストの確認に答えられない。
// 画面は保存の前にこれを尋ね、接続と同じ照合で警告を出す。
func (h HostKeys) Knows(target Target) (bool, error) {
	entries, err := h.entriesFor(target)
	if err != nil {
		return false, err
	}
	for _, found := range entries {
		if found.entry.Marker == "" {
			return true, nil
		}
	}
	return false, nil
}

// hostEntry は、このホストについて known_hosts に書かれた行ひとつである。
type hostEntry struct {
	at    knownHostsLocation
	entry *knownhosts.Entry
}

// entriesFor は、照合に使うすべての known_hosts から、このホストについての行を
// OpenSSH が読む順（UserKnownHostsFile、GlobalKnownHostsFile）に集める。
func (h HostKeys) entriesFor(target Target) ([]hostEntry, error) {
	if target.KnownHosts.ExpansionError != nil {
		return nil, target.KnownHosts.ExpansionError
	}
	if h.Read == nil {
		return nil, nil
	}
	field := knownHostsField(target)
	var found []hostEntry
	for _, path := range target.KnownHosts.all() {
		contents, err := h.Read(path)
		if err != nil {
			return nil, err
		}
		for _, line := range knownhosts.ParseFile(contents).Lines {
			if line.Entry != nil && line.Entry.MatchesHost(field) {
				found = append(found, hostEntry{at: knownHostsLocation{path: path, number: line.Number}, entry: line.Entry})
			}
		}
	}
	return found, nil
}

// signatureAlgorithms は、known_hosts が書く鍵の種類を、交渉で名乗る署名
// アルゴリズムへ広げる。
//
// RSA だけは、鍵の種類と署名アルゴリズムが一対一ではない。known_hosts は
// ssh-rsa としか書かないが、それをそのまま名乗ると SHA-1 の署名だけを求める
// ことになり、SHA-1 を断る今どきのサーバーとは繋がらない。同じ鍵で名乗れる
// SHA-2の二つをOpenSSHと同じ順で返す。SHA-1のssh-rsaは利用者が
// HostKeyAlgorithmsで明示した接続だけに限る。
func signatureAlgorithms(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{keyType}
}

// verify は、最初の鍵交換では提示された鍵を known_hosts と照合して受け入れた鍵を
// 覚え、鍵の再交換（rekey）では覚えた鍵と比べる。
func (lookup *hostKeyLookup) verify(key ssh.PublicKey, prompt Prompter) (hostKeyVerdict, error) {
	lookup.mutex.Lock()
	defer lookup.mutex.Unlock()
	if lookup.firstKey != nil {
		if !bytes.Equal(lookup.firstKey, key.Marshal()) {
			return hostKeyVerdict{}, ErrHostKeyChangedDuringRekey
		}
		return hostKeyVerdict{outcome: hostKeyUnchanged}, nil
	}
	verdict, err := lookup.verifyAgainstKnownHosts(key, prompt)
	if err == nil {
		lookup.firstKey = key.Marshal()
	}
	return verdict, err
}

// verifyAgainstKnownHosts は、提示された鍵を known_hosts のすべてのファイルと照合する。
//
// OpenSSH と同じく、どのファイルのどの行でも @revoked の同じ鍵があれば断り、
// 印の無い同じ鍵があれば既知とし、同じホストに別の鍵しか無ければ変わった鍵として
// 断る。組織が GlobalKnownHostsFile で配った鍵は、ここで照合される。
func (lookup *hostKeyLookup) verifyAgainstKnownHosts(key ssh.PublicKey, prompt Prompter) (hostKeyVerdict, error) {
	offered := base64.StdEncoding.EncodeToString(key.Marshal())
	var verdict hostKeyVerdict
	entries, err := lookup.hostEntries()
	if err != nil {
		return verdict, err
	}
	for _, found := range entries {
		if strings.EqualFold(found.entry.Marker, "@revoked") && found.entry.Key == offered {
			return verdict, ErrHostKeyRevoked
		}
	}
	for _, found := range entries {
		// @cert-authority は、その鍵で署名された証明書を認めるという意味であり、
		// ホスト鍵そのものではない。証明書はこのクライアントがまだ扱わないので、
		// 一致の判断からは外す。
		if found.entry.Marker != "" {
			continue
		}
		if found.entry.Key == offered {
			return hostKeyVerdict{outcome: hostKeyKnown, matched: found.at}, nil
		}
		verdict.conflicting = append(verdict.conflicting, conflictingHostKey{
			at: found.at, key: found.entry.KeyType + " " + found.entry.Fingerprint,
		})
	}
	if len(verdict.conflicting) > 0 {
		return verdict, ErrHostKeyChanged
	}
	verdict.outcome, verdict.rememberedIn, err = lookup.keys.accept(lookup.target, key, offered, prompt)
	return verdict, err
}

// accept は、未知のホストをどう扱うかを StrictHostKeyChecking で決める。受け入れた
// 鍵を書いたファイルも返す。target.Strict は NewTarget が yes・no・accept-new・ask に
// 揃えてある。
func (h HostKeys) accept(target Target, key ssh.PublicKey, offered string, prompt Prompter) (hostKeyOutcome, string, error) {
	switch target.Strict {
	case "yes":
		return 0, "", ErrHostKeyUnknown
	case "no", "accept-new":
		rememberedIn, err := h.remember(target, key, offered)
		return hostKeyTrusted, rememberedIn, err
	}

	if prompt == nil {
		return 0, "", ErrHostKeyUnknown
	}
	accepted, err := prompt.Confirm(UnknownHostPrompt(target, key))
	if err != nil {
		return 0, "", err
	}
	if !accepted {
		return 0, "", ErrHostKeyUnknown
	}
	rememberedIn, err := h.remember(target, key, offered)
	return hostKeyConfirmed, rememberedIn, err
}

// remember は、受け入れた鍵を UserKnownHostsFile の最初のファイルへ書き、書いた
// ファイルを返す。UserKnownHostsFile が none のときと、sshc が書かない場所
// （~/.ssh の外、/dev/null）のときは書かずに空を返す。~/.ssh の中でもリンクを
// 経由するファイルには書けず、KnownHostsSymlinkError で接続を失敗にする。
func (h HostKeys) remember(target Target, key ssh.PublicKey, offered string) (string, error) {
	if h.Add == nil || len(target.KnownHosts.User) == 0 {
		return "", nil
	}
	fingerprint, err := knownhosts.Fingerprint(offered)
	if err != nil {
		return "", err
	}
	host, port := target.HostName, knownhosts.ParsePort(target.Port)
	if target.HostKeyAlias != "" {
		// OpenSSH は HostKeyAlias をポートなしの名前として書く。
		host, port = target.HostKeyAlias, knownhosts.DefaultPort
	}
	path := target.KnownHosts.User[0]
	err = h.Add(path, knownhosts.Candidate{
		Host: host, Port: port, KeyType: key.Type(), Key: offered, Fingerprint: fingerprint,
	})
	switch {
	case errors.Is(err, knownhosts.ErrNotWritable):
		return "", nil
	case errors.Is(err, knownhosts.ErrSymlinkPath):
		return "", &KnownHostsSymlinkError{Path: path, Err: err}
	case err != nil:
		return "", err
	}
	return path, nil
}

// UnknownHostPrompt は、OpenSSH が同じ場面で出す問いに寄せた文言である。
//
// フィンガープリントを見せるのは、そのユーザーが別の経路で確かめられるようにするため
// である。見せずに「続けますか」とだけ尋ねるのは、確かめる手段を持たない問いである。
func UnknownHostPrompt(target Target, key ssh.PublicKey) string {
	fingerprint := ssh.FingerprintSHA256(key)
	where := target.HostName
	if target.Alias != "" && target.Alias != target.HostName {
		where = target.Alias + " (" + target.HostName + ")"
	}
	return fmt.Sprintf(
		"The authenticity of host '%s' cannot be established.\r\n"+
			"%s key fingerprint is %s.\r\n"+
			"Are you sure you want to continue connecting (yes/no)? ",
		where, strings.ToUpper(key.Type()), fingerprint)
}

// knownHostsField は、known_hosts でこのホストを探す名前である。
//
// HostKeyAlias があればそれをそのまま使う。無ければ HostName と Port から
// internal/knownhosts の HostField が作る。鍵を書く Service も同じ関数を使うので、
// 受け入れて書いた鍵に次の接続が一致する。
func knownHostsField(target Target) string {
	if target.HostKeyAlias != "" {
		return target.HostKeyAlias
	}
	return knownhosts.HostField(target.HostName, knownhosts.ParsePort(target.Port))
}
