# リリース運用

`scripts/release/publish.sh`は、main CIの待機からタグ作成、保護ゲート承認、公開後検証までを一つのコマンドで実行します。ビルド自体はGitHub Actions上で継続しますが、runの監視と承認、成果物の手動確認は不要です。

## 事前条件

- リリース対象を`origin/main`へpush済みで、作業treeがcleanであること
- `docs/releases/<tag>.md`を追加済みであること
- stable releaseではREADME、`docs/release-install.md`、`install.sh`、`pages/guide/install.md`、`pages/en/guide/install.md`で固定したバージョンが同じtagであること（`internal/buildcontract` の契約テストが照合する）
- 公開するcommitで、`release-ui-check.yml`の3つのjobが成功していること（次の「公開の前に埋め込みUIの照合を試す」）
- `gh auth status`が成功し、repositoryと`release` environmentを操作できること
- `git`、`gh`、`jq`、`curl`、`unzip`、`sha256sum`または`shasum`が利用できること

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

## 公開

```sh
scripts/release/publish.sh v0.33.2
```

スクリプトは次を順番に行います。

1. HEAD、`origin/main`、同じSHAのmain CI成功を照合
2. 注釈付きtagを作成してpush
3. Release workflowを検出し、`release` environmentだけを承認
4. workflowの全jobが成功するまで状態変化を表示
5. Immutable Release、9成果物、checksums、全attestation、APK、実行可能なnative binary、release本文を検証
6. stable releaseではHomebrew Formulaのtagとsource SHA-256を検証

main CIまたはRelease workflowが失敗した場合、tagを動かしたり削除したりせず終了します。原因を修正して新しいpatch versionを作成してください。

## 公開済みReleaseの再検証

```sh
scripts/release/publish.sh --verify-only v0.17.3
```

このモードはtagやGitHub上の状態を変更せず、公開成果物とHomebrew tapだけを再検証します。
