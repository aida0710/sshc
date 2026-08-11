# 答えられないプロンプトを人間へ中継する

パスワードを保存すると、そのホストへ接続できなくなることがある。

`connect.go` は保存済みパスワードのある alias に対してだけ `SSH_ASKPASS_REQUIRE=force`
を立てる。`force` は「端末があっても、対話的な問いはすべて askpass へ回す」という
意味なので、ホスト鍵の確認までヘルパーに届く。ヘルパーはそれを断る。断るのは正しい
— その問いに自動で答えるヘルパーは検査を取り除いてしまう — が、断った先に何もない
ため、利用者は端末に戻る以外の道を失う。

パスワードを保存していなければ、素の ssh が端末で普通に訊いてきて、`yes` と打てば
済んでいた。**保存という操作が、答える手段を奪っている。**

これはホスト鍵に限らない。`refusalMarkers` はこう並んでいる。

```
"passphrase", "continue connecting", "fingerprint",
"yes/no", "verification code", "one-time", "token"
```

鍵のパスフレーズも、2FA のワンタイムコードも同じく断られる。ホスト鍵は、その一族の
うち最初に表へ出た 1 つにすぎない。

## 契約を変える

> **今**: パスワードには答える。それ以外は断る。
> **後**: パスワードには答える。それ以外は**人間に中継する**。

`SSH_ASKPASS_REQUIRE=force` によってすべての問いを通されるヘルパーが取るべき姿勢は、
「答えられないものは握りつぶす」ではなく「答えられないものは持ち主に渡す」である。

## 中継できる根拠

実験で確かめた。`SSH_ASKPASS_REQUIRE=force` の下でヘルパーに渡されるのは、
プロンプト本文の全体である。そして stdin はパイプだが、**`/dev/tty` は読める**。

```
PROMPT: The authenticity of host '100.88.32.120 ...' can't be established.
        ED25519 key fingerprint is: SHA256:PJfawn3ikvzrqo7nENLU3P83u5j1zb+SYeshHR8tOmk
        Are you sure you want to continue connecting (yes/no/[fingerprint])?
TTY: readable
stdin NOT a tty
```

接続は必ず端末の中で起きる。`sshc <alias>` も、画面の Connect から開く Terminal も、
最後は同じ `syscall.Exec` を通る。だから制御端末は常にある。

## 流れ

```
ssh ──(prompt)──> helper
                    ├─ AnswerablePrompt が true → POST /askpass → 保存済みパスワード → stdout
                    ├─ false → /dev/tty を開く → プロンプトを出す → 1 行読む → stdout
                    └─ tty が無い → 従来どおりメッセージを出して非ゼロ終了
```

`AnswerablePrompt` は 1 文字も変えない。あれは「保存済みパスワードを返してよいか」の
許可リストであり、そこが境界である。中継はその外側の別経路で、返せるのは**人間が
打った文字列だけ**である。yes/no の問いにアカウントのパスワードが渡る経路は生まれない。

`refusalMarkers` の意味は「拒否する語」から「自動では答えない語」へ変わる。並びは
変えない。

## エコー

ホスト鍵の確認だけ表示し、それ以外は伏せる。判定は `passwordPromptSuffix` と同じ
サフィックス照合で、`(yes/no/[fingerprint])? ` を見る。

OpenSSH が文言を変えたら**伏せる側に倒れる**。最悪でも `yes` が見えないだけで、
パスフレーズが画面に残ることはない。失敗の向きをそちらに選ぶ。

`golang.org/x/term` は既に直接依存にあるので、依存は増えない。

## 自前のダイアログを作らない

A（同じ鍵が別名で既知）と B（初見の鍵）の区別は、OpenSSH 自身がプロンプト本文で
やっている。

```
A: This host key is known by the following other names/addresses:
       ~/.ssh/known_hosts:956: 10.0.10.161
B: This key is not known by any other names.
```

そのまま見せる方が、自前の文言より情報が多く、保守するものが増えない。ヘルパーは
プロンプトを解析しない — これは `AliasVariable` の doc が既に述べている姿勢であり、
ここでも守る。例外はエコー判定のサフィックス照合ひとつだけで、外したときに安全側へ
倒れるように作る。

## 継ぎ目

端末はインターフェースの向こうに置く。

```go
// terminalPrompter は、人間に問い、その答えを返す。
type terminalPrompter interface {
	Prompt(question string, echo bool) (string, error)
	Close() error
}
```

`runAskpass` は `open func() (terminalPrompter, error)` を受け取る。本物は `/dev/tty`
を開き、`echo` が偽なら `term.ReadPassword` を使う。テストは記録用の実装を差し込み、
**本物の端末には触れない**。`chooseTUIHost` が `*os.File` を受け取っているのと同じ
考え方である。

## 境界

- 画面のダイアログは作らない。サーバーも API も変えない。
- `known_hosts` を書くのは今までどおり ssh である。sshc は書かない。
- Known Hosts 画面は変えない。初見の鍵を fingerprint で検証したい人の道はそこに残る。
- vault が施錠されている等でパスワードを引き換えられなかった場合は、従来どおり
  メッセージを出して非ゼロで終わる。中継するのは `AnswerablePrompt` が偽のときだけ
  である。引き換えの失敗まで中継に流すと、「保存したはずのパスワードが使われなかった」
  ことに利用者が気づけなくなる。
- tty が開けない起動（systemd や launchd から `-open=false` で上がったエージェント）
  では、従来どおり断る。そこには答える人間がいない。
