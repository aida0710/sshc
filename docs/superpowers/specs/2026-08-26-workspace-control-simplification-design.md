# Workspace操作整理 設計

## 目的

ターミナル本文へ管理ボタンを重ねず、スマホでは接続先の切り替えと端末操作に集中できる画面にする。分割作成はDrag & Dropへ一本化し、一括操作は誤入力しにくい確認付きフローにする。

## 用語と状態

- **Live Workspace**: 現在接続中のterminal sessionを一時的にまとめた表示状態。Drag & Dropで自動的に作られ、session、process、scrollbackを維持する。
- **保存レイアウト**: 接続先aliasとpane tree、分割比率、focus位置を名前付きで永続化した再接続用preset。開くと新しいSSH接続を作成し、sessionとscrollbackは復元しない。
- 1 Workspaceは最大4 paneとする。新しいpaneの追加はUI reducerと保存domainの両方で拒否し、既存paneの移動は許可する。

## デスクトップ

- 接続一覧のterminalを表示中paneの上下左右へDropして分割する。Split専用ボタンは置かない。
- 各paneの上に固定高のタイトルバーを置き、Drag handle、alias、Focus、Workspaceから外す操作を配置する。terminal surfaceへabsolute overlayしない。
- 全体toolbarは「一括実行…」と「保存レイアウト」の2入口に絞る。
- 保存レイアウトのpopoverに、何を保存するか、再度開いたときに新しい接続になること、4 pane上限を明記する。

## スマホ

- 全体toolbarとpaneタイトルバーを表示しない。
- Workspaceが複数接続を含む場合も、terminalはfocus中の1 paneだけを描画する。
- 接続切替tabだけをterminal上部へ表示する。分割、resize、Focus、保存、一括実行は表示しない。

## 一括実行

- live keystrokeの複製は廃止する。
- 「一括実行…」でmodalを開き、直接commandまたはSnippet、hostごと／paneごとの対象、previewを順に選択する。
- 実行は既存の独立SSH execution APIを使う。表示中terminalのcwd、環境変数、shell状態は継承しない。
- modalは`role=dialog`、`aria-modal=true`とし、Escape、閉じるbutton、backdropで閉じられる。

## 検証

- reducer: 4 paneへの追加成功、5 pane目の拒否、上限到達後の既存pane移動。
- domain: 5 paneの保存拒否。
- component: モバイルでtoolbar／pane toolbar非表示、5台目の説明、modalのEscape終了。
- E2E: Drag分割、タイトルバー、保存レイアウト、Broadcast modal、360×800の単一paneと横overflow。
