---
title: Push・Pull・履歴
description: Git風の同期操作、自動同期、競合、force操作を理解する。
---

# Push・Pull・履歴

sshcの同期は自動mergeを行いません。localとremoteのrevisionを比較し、何を送る／受け取るかを明示します。

## 通常の流れ

1. **Bucketの状態を更新**してlive objectと履歴を取得
2. **変更を確認**して追加、変更、削除をpreview
3. localを正とするなら**Push**、remoteを正とするなら**Pull**
4. conflictや削除がある場合は内容を確認して明示操作

`sshc sync now`は設定された方向に従い、安全に自動判断できる操作だけを行います。

## 自動同期

自動同期はremoteを定期確認しますが、1分ごとに無条件uploadはしません。local内容がsshc経由または外部editorで変わった場合にdigestを比較し、必要なときだけpushします。remoteが進んでいる場合はpullまたは停止判断を行います。

## Force操作

force push／pullは「最後にpreviewしたexact ETagとrevision」にだけ作用する短命tokenを要求します。別端末がその後remoteを更新していれば拒否されます。bucketを途中で変更した場合も、古い確認結果を新しい保存先へ流用しません。

## 履歴

sshcはS3 bucketを直接確認し、live snapshotと日付付き履歴をgraphとして表示します。履歴の一覧表示は暗号文を復号せずに取得できますが、内容diffや復元にはsync keyが必要です。

## CLI

```sh
sshc sync
sshc sync push --json
sshc sync pull --json
sshc sync now --json
sshc sync auto on
```

CIやheadless hostでは`--json`を使い、exit codeと単一JSON objectの両方を確認してください。
