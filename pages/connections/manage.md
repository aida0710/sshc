---
title: 接続とグループ
description: 多数のSSH接続を検索、分類、編集する。
---

# 接続とグループ

![3列で接続を管理する画面](/images/connections-management.png)

Connectionsはgroup tree、接続一覧、詳細の3列で構成されます。狭い画面では一列ずつ表示し、browserの戻る操作で前の層へ戻れます。

## 接続を探す

一覧は既定で名前順です。検索はalias、接続先address、user、groupを対象にします。`Ctrl/Cmd+K`を使うと、現在のpageを離れずにhost、remote file、Snippet、設定を横断検索できます。host結果のメニューから詳細設定へ直接移動できます。

HomeのQuick accessは最近接続した順を優先し、未接続の項目を名前順で続けます。表示はpanel／listから選べ、選択はbrowserへ保存されます。

## Group

groupは接続を分類するdirectoryです。dragで階層化や並べ替えができます。groupの変更はpreview後に保存し、対応する`Include`構造へ反映されます。

## 作成、複製、名前変更

aliasは一意です。既に同名の設定がある場合、作成、複製、名前変更は保存前に拒否されます。削除や移動では、設定fileと関連するsshc管理データを確認してから実行してください。

## Quick accessとConnectionsの違い

- **Quick access**: すぐ接続するための入口
- **Connections**: 大量の接続を検索、分類、解析、編集する管理画面

Quick accessから接続先のメニューを開くとConnectionsの詳細へ移動できます。
