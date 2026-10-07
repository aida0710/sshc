# リリース運用

`scripts/release/publish.sh`は、main CIと埋め込みUIの照合（`release-ui-check.yml`）の待機から、タグの作成、`release` environmentの承認、公開後の検証までを1つのコマンドで実行します。ビルドはGitHub Actionsで進みますが、runの監視と承認、成果物の手動確認は不要です。publish.shを実行することが、その公開の承認になります（`docs/release-install.md`の「リポジトリ管理者向けの公開保護」）。

## 事前条件

- リリースするコミットを`origin/main`へpush済みで、作業ツリーに未コミットの変更がないこと
- `docs/releases/<tag>.md`を追加済みであること
- 安定バージョンのリリースでは、`scripts/release/check-pinned-installers.sh`が照合する導入例（README、`docs/release-install.md`のPowerShellの例など）で固定したバージョンがタグと同じであること
- 公開するcommitで、`release-ui-check.yml`の3つのjobが成功していること（次の「公開の前に埋め込みUIの照合を試す」）
- VPNコンテナのイメージが固定したUbuntuのsnapshotの時刻（`internal/vpn/container/Dockerfile`の`snapshot=`）が30日より古ければ、先に上げてmainへ入れてあること。固定した時刻より後のセキュリティ修正はイメージに入らず、Dockerfileを書き換えない限り利用者のマシンのイメージも作り直されない。baseのdigestとsuiteごとのInReleaseのSHA-256を同じ変更で書き換え（手順はDockerfileのコメント）、CIの`vpn-image` jobでamd64とarm64の両方を確かめる。`publish.sh`は30日より古いと警告を出す（失敗にはしない）
- `gh auth status`が成功し、リポジトリと`release` environmentを操作できること
- `git`、`gh`、`jq`、`curl`、`unzip`、`sha256sum`または`shasum`が利用できること

導入例を照合する文書の一覧は、`scripts/release/check-pinned-installers.sh`にだけ書かれています。`publish.sh`とReleaseワークフローは、公開前にこのスクリプトを公開するタグで実行します。`internal/buildcontract`の契約テストは、`docs/releases`にある最新の安定バージョンのリリースノートのバージョンで同じスクリプトを実行します。このため、リリースノートを追加する変更で導入例のバージョンも更新してください。導入例を置く文書を増やしたときは、このスクリプトの一覧にも追加してください。

## 公開の前に埋め込みUIの照合を試す

Release workflowのmacOS・Linux・Windowsのjobは、runner上で埋め込みUI（`internal/ui/dist`）を作り直し、コミット済みのものと違えばバイナリを作らずに失敗します。UIのビルドの出力がOSによって違うと、公開の当日にReleaseが止まります。

`release-ui-check.yml`は、同じrunnerでこの照合だけを行うworkflowです。バイナリは作らず、公開・署名・tagの権限も持ちません。公開するcommitを`origin/main`へpushしたら、公開の前に次を実行してください。

```sh
gh workflow run release-ui-check.yml --ref main
gh run list --workflow release-ui-check.yml --branch main --limit 1 --json databaseId,headSha,status
gh run watch <databaseId> --exit-status
```

`gh run list`に新しいrunが出るまで、数秒かかることがあります。`headSha`が公開するcommitと同じであることを確認してください。

3つのjobがすべて成功したら、公開へ進んでください。失敗したjobのログには、コミット済みのものと違ったファイルが`git status --porcelain`の形式で表示されます。その場合は公開せず、差分の原因を調べてください。

`publish.sh`も、公開するcommitでこのworkflowが成功したことを、main CIと同じやり方（同じSHAで`main`から走ったrunのうち、いちばん新しいもの）で確かめます。runが無ければ、main CIを待つ前にタグを作らずに終了します。runが実行中なら終わるまで待ち、失敗していればタグを作らずに終了します。

## 公開

```sh
scripts/release/publish.sh v0.33.2
```

スクリプトは次を順番に行います。

1. HEADと`origin/main`が同じで、同じSHAでmain CIと`release-ui-check.yml`が成功したことを照合
2. 注釈付きタグを作成してpush
3. Releaseワークフローのrunを見つけ、`release` environmentだけを実行者の認証情報で承認（確認の入力は求めない）
4. ワークフローのすべてのjobが成功するまで、状態の変化を表示
5. Immutable Release、11個の成果物（デモ一式のtar.gzとmanifestを含む）、`checksums.txt`、すべてのattestation、APK、このマシン向けのバイナリが報告するバージョン、Releaseの本文を検証
6. 安定バージョンでは、Homebrewのformulaが指すタグとソースのSHA-256を検証

main CI、`release-ui-check.yml`、Releaseワークフローのどれかが失敗した場合、タグを動かしたり削除したりせず終了します。対処は次の「失敗したとき」を参照してください。

## 失敗したとき

止まった場所と原因で対処が変わります。新しいパッチバージョンが必要になるのは、タグのコミット自体に不具合がある場合だけです。

### タグをpushする前に止まった場合

次の場合、タグはまだ作成されていません。それぞれの対処をしてから、同じバージョンで`scripts/release/publish.sh <tag>`をもう一度実行します。

- main CIの失敗: mainを直してpushする
- `release-ui-check.yml`のrunが無い: 「公開の前に埋め込みUIの照合を試す」の手順で走らせる
- `release-ui-check.yml`の失敗: 失敗したjobのログで、コミット済みのものと違ったファイルを調べて直す
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

元のrunを再実行できない場合は、`main`のReleaseワークフローを同じタグで実行します。ワークフローは、タグのコミットが`main`の祖先で、同じSHAでmain CIが成功していることを確かめてから作り直します。

```sh
gh workflow run release.yml --repo aida0710/sshc --ref main -f tag=<tag>
```

`release` environmentの承認を求められたら、runの画面で承認します。ワークフローが成功したら、公開した成果物を検証します。

```sh
scripts/release/publish.sh --verify-only <tag>
```

手動で実行したReleaseワークフローは、より新しい安定バージョンが公開済みでも、古い安定バージョンを作り直せます。その場合、GitHubのlatestとHomebrew tapは新しい方を指したままです。`--verify-only`は成果物、チェックサム、attestationを検証したあと、Homebrew tapの照合で止まります。

latestにする安定バージョンでは、Releaseワークフローがタグのコミットにある`scripts/release/check-pinned-installers.sh`で導入例を照合します。このスクリプトを含まない古いコミットのタグは照合できないため、ワークフローは理由を表示して止まります。

### タグのコミットに不具合がある場合

タグは動かせないため、原因を修正して新しいパッチバージョンを作成してください。

## 公開済みReleaseの再検証

```sh
scripts/release/publish.sh --verify-only v0.17.3
```

このモードはタグやGitHub上の状態を変更せず、公開成果物とHomebrew tapだけを再検証します。
