# リリース運用

`scripts/release/publish.sh`は、main CIの待機からタグ作成、保護ゲート承認、公開後検証までを一つのコマンドで実行します。ビルド自体はGitHub Actions上で継続しますが、runの監視と承認、成果物の手動確認は不要です。

## 事前条件

- リリース対象を`origin/main`へpush済みで、作業treeがcleanであること
- `docs/releases/<tag>.md`を追加済みであること
- stable releaseではREADME、`docs/release-install.md`、`install.sh`の固定版が同じtagであること
- `gh auth status`が成功し、repositoryと`release` environmentを操作できること
- `git`、`gh`、`jq`、`curl`、`unzip`、`sha256sum`または`shasum`が利用できること

## 公開

```sh
scripts/release/publish.sh v0.26.2
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
