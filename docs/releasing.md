# リリース運用

`scripts/release/publish.sh`は、main CIの待機から、タグの作成、`release` environmentの承認、公開後の検証までを1つのコマンドで実行します。ビルドはGitHub Actionsで進みますが、runの監視と承認、成果物の手動確認は不要です。publish.shを実行することが、その公開の承認になります（`docs/release-install.md`の「リポジトリ管理者向けの公開保護」）。

## 事前条件

- リリースするコミットを`origin/main`へpush済みで、作業ツリーに未コミットの変更がないこと
- `docs/releases/<tag>.md`を追加済みであること
- 安定バージョンのリリースでは、`scripts/release/check-pinned-installers.sh`が照合する導入例（README、`docs/release-install.md`のPowerShellの例など）で固定したバージョンがタグと同じであること
- VPNコンテナのイメージが固定したUbuntuのsnapshotの時刻（`internal/vpn/container/Dockerfile`の`snapshot=`）が30日より古ければ、先に上げてmainへ入れてあること。固定した時刻より後のセキュリティ修正はイメージに入らず、Dockerfileを書き換えない限り利用者のマシンのイメージも作り直されない。baseのdigestとsuiteごとのInReleaseのSHA-256を同じ変更で書き換え（手順はDockerfileのコメント）、CIの`vpn-image` jobでamd64とarm64の両方を確かめる。`publish.sh`は30日より古いと警告を出す（失敗にはしない）
- 前のリリースから実行先との互換の番号を上げた場合は、リリースノートの「更新後に必要な操作」に、実行先の更新が必要なことを書いてあること（次の「実行先との互換の番号」）
- `gh auth status`が成功し、リポジトリと`release` environmentを操作できること
- `git`、`gh`、`jq`、`curl`、`unzip`、`sha256sum`または`shasum`が利用できること

導入例を照合する文書の一覧は、`scripts/release/check-pinned-installers.sh`にだけ書かれています。`publish.sh`とReleaseワークフローは、公開前にこのスクリプトを公開するタグで実行します。`internal/buildcontract`の契約テストは、`docs/releases`にある最新の安定バージョンのリリースノートのバージョンで同じスクリプトを実行します。このため、リリースノートを追加する変更で導入例のバージョンも更新してください。導入例を置く文書を増やしたときは、このスクリプトの一覧にも追加してください。

## 実行先との互換の番号

このマシンのsshcエンジンは、実行先（SSHの接続先で動くsshcエンジン）を、sshcのバージョンではなく互換の番号で照合します。番号は`internal/remoteengine/remote_engine.go`の`Compatibility`の1か所にだけあり、sshcエンジンはhandoffの`runTargetCompatibility`に書きます。番号が同じなら、バージョンが違っても実行先として使えます。

次のどれかを、古いsshcと通じない形に変えた変更では、同じ変更で`Compatibility`を1つ上げてください。

- 実行先へ取り次ぐAPI（`/api/v1/terminal/…`）の要求と答えの形
- handoffと`/cli/`の手順（資格の受け取り方、身元の確かめ方）
- ターミナルの入出力のストリームの形

上げたリリースでは、実行先も同じリリース以降へ更新しないと接続できません。リリースノートの「更新後に必要な操作」に、実行先の更新が必要なことと、接続先で`sshc update`を実行することを書いてください。番号を上げていないリリースでは、実行先の更新は必須ではありません。

## 配布物に埋め込む画面

組み立てたWeb UI（`internal/ui/dist/app`）はリポジトリに入れていません。Release workflowの`ui` jobが画面を1回だけ組み立て、macOS・Linux・Windows・Androidのjobは同じものを受け取って埋め込みます。OSごとに組み立てると、ネイティブの依存やNodeの違いで、同じタグの配布物が別々の画面を持ちうるためです。nativebuildの`release-current`と`matrix`は画面を作り直さず、置かれていなければバイナリを作らずに失敗します。

Homebrewのformulaは、タグのソースからNode.jsで画面を組み立ててからビルドします（`packaging/homebrew/sshc.rb`）。

## 公開

```sh
scripts/release/publish.sh v0.33.2
```

スクリプトは次を順番に行います。

1. HEADと`origin/main`が同じで、同じSHAでmain CIが成功したことを照合
2. 注釈付きタグを作成してpush
3. Releaseワークフローのrunを見つけ、`release` environmentだけを実行者の認証情報で承認（確認の入力は求めない）
4. ワークフローのすべてのjobが成功するまで、状態の変化を表示
5. Immutable Release、11個の成果物（デモ一式のtar.gzとmanifestを含む）、`checksums.txt`、すべてのattestation、APK、このマシン向けのバイナリが報告するバージョン、Releaseの本文を検証
6. 安定バージョンでは、Homebrewのformulaが指すタグとソースのSHA-256を検証

main CIかReleaseワークフローが失敗した場合、タグを動かしたり削除したりせず終了します。対処は次の「失敗したとき」を参照してください。

## 失敗したとき

止まった場所と原因で対処が変わります。新しいパッチバージョンが必要になるのは、タグのコミット自体に不具合がある場合だけです。

### タグをpushする前に止まった場合

次の場合、タグはまだ作成されていません。それぞれの対処をしてから、同じバージョンで`scripts/release/publish.sh <tag>`をもう一度実行します。

- main CIの失敗: mainを直してpushする
- CIを待つあいだの`origin/main`の更新: 公開するcommitを選び直す
- タグのpushの失敗: 原因を取り除く。publish.shは、作成したローカルのタグを削除してから終了します

### タグのpush後にReleaseワークフローが始まらない場合

publish.shは、タグをpushしてから約1分のあいだにReleaseワークフローのrunが見つからないと終了します。GitHubがタグのpushを受けてワークフローを起動するのが遅れたか、起動しなかった場合です。タグはpush済みなので、publish.shを同じバージョンで実行し直してもタグの確認で止まります。

まず、遅れて始まったrunがないかを確かめます。

```sh
gh run list --repo aida0710/sshc --workflow release.yml --branch <tag>
```

runがあれば、`release` environmentの承認を求められたときにrunの画面で承認します。runがなければ、次の節と同じく`main`のReleaseワークフローを同じタグで実行します。どちらの場合も、ワークフローが成功したら`scripts/release/publish.sh --verify-only <tag>`で検証します。

### Releaseワークフローが一時的な理由で失敗した場合

runner、ネットワーク、`sum.golang.org`などの一時的な障害で失敗し、タグのコミットに問題がない場合は、同じタグで作り直します。Releaseワークフローは、同じタグの未公開のdraftだけを捨てて作り直し、公開済みのReleaseには触れません。

失敗したjobだけを再実行します。

```sh
gh run rerun <run-id> --failed --repo aida0710/sshc
```

jobのあいだで受け渡す成果物（組み立てた画面と各OSの成果物）は1日で消えます。最初の実行から1日を過ぎたら、失敗したjobだけの再実行は成果物を受け取れずに失敗するので、次の`main`のReleaseワークフローで作り直してください。

元のrunを再実行できない場合は、`main`のReleaseワークフローを同じタグで実行します。ワークフローは、タグのコミットが`main`の祖先で、同じSHAでmain CIが成功していることを確かめてから作り直します。

```sh
gh workflow run release.yml --repo aida0710/sshc --ref main -f tag=<tag>
```

`release` environmentの承認を求められたら、runの画面で承認します。ワークフローが成功したら、公開した成果物を検証します。

```sh
scripts/release/publish.sh --verify-only <tag>
```

手動で実行したReleaseワークフローは、より新しい安定バージョンが公開済みでも、古い安定バージョンを作り直せます。ただし、組み立てた画面をリポジトリに入れていたv0.54.0までのタグは、画面の出力先が違うため作り直せません。その場合、GitHubのlatestとHomebrew tapは新しい方を指したままです。`--verify-only`は成果物、チェックサム、attestationを検証したあと、Homebrew tapの照合で止まります。

latestにする安定バージョンでは、Releaseワークフローがタグのコミットにある`scripts/release/check-pinned-installers.sh`で導入例を照合します。このスクリプトを含まない古いコミットのタグは照合できないため、ワークフローは理由を表示して止まります。

### タグのコミットに不具合がある場合

タグは動かせないため、原因を修正して新しいパッチバージョンを作成してください。

## 公開済みReleaseの再検証

```sh
scripts/release/publish.sh --verify-only v0.17.3
```

このモードはタグやGitHub上の状態を変更せず、公開成果物とHomebrew tapだけを再検証します。
