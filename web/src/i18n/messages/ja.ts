import type { MessageKey } from "./en";

export const ja = {
  "shell.title": "sshc",
  "shell.starting": "ローカルセッションを開始しています…",
  "shell.vaultChecking":
    "保護された内容を再表示する前に、Vault のロック状態を確認しています…",
  "shell.vaultCheckRetrying":
    "Vault の状態を確認できませんでした。保護された内容を隠したまま再試行します。",
  "shell.active": "ローカルセッション稼働中 · {version}",
  "shell.bootstrapFailed":
    "ローカルセッションを開始できませんでした。ターミナルで sshc を一度実行し、このブラウザーを登録し直してください。",
  "shell.bootstrapRetry": "もう一度試す",
  "shell.sessionEndedHeading": "セッションが終了しました",
  "shell.sessionEnded":
    "再読み込みするとローカルセッションを自動で復旧します。このブラウザーの登録が無効な場合だけ、ターミナルで sshc を一度実行してください。",
  "shell.sessionReload": "セッションを再読み込み",
  "shell.pageNotFound": "ページが見つかりません",
  "shell.pageNotFoundDescription":
    "この URL に対応する sshc の画面はありません。",
  "shell.goHome": "ホームへ移動",
  "shell.primaryNavigation": "メインナビゲーション",
  "shell.navigationToggle": "ナビゲーション",
  "shell.navigationResize": "ナビゲーションの幅を変更",
  "shell.sessions": "Sessions",
  "shell.navStart": "Main",
  "shell.navConnections": "Configuration",
  "shell.navKeysHosts": "Security",
  "shell.navMaintenance": "Tools",
  "menu.open": "{section}を開く",
  "menu.displaySettings": "Preferences",
  "menu.theme": "Theme",
  "menu.language": "Language",
  "shell.inspectorShowNamed": "{label}を表示",
  "shell.inspectorHideNamed": "{label}を隠す",
  "shell.inspectorAttention": "確認が必要な項目があります",
  "palette.open": "Search",
  "palette.heading": "セッション・ホスト・ファイル・スニペット・設定を検索",
  "palette.placeholder": "セッション、ホスト、ファイル、スニペット、設定を検索",
  "palette.results": "検索結果",
  "palette.loading": "検索項目を読み込んでいます…",
  "palette.empty": "一致する項目がありません",
  "palette.hint": "↑↓ 選択 · Enter 開く · Esc 閉じる",
  "palette.connectHost": "{alias} へ接続",
  "palette.openHostSettings": "{alias} の接続設定を開く",
  "palette.openSection": "セクションを開く",
  "palette.kind.session": "セッション",
  "palette.kind.host": "ホスト",
  "palette.kind.file": "ファイル",
  "palette.kind.snippet": "スニペット",
  "palette.kind.setting": "設定",
  "table.sortAscending": "、昇順に並べ替え",
  "table.sortDescending": "、降順に並べ替え",
  "section.files": "SFTP",
  "section.snippets": "Snippets",
  "sftp.heading": "リモートファイル",
  "sftp.host": "ホスト",
  "sftp.noHosts": "保存済みホストなし",
  "sftp.chooseHost": "ホストを選択",
  "sftp.connectionFailed": "接続できませんでした。",
  "sftp.path": "リモートパス",
  "sftp.go": "移動",
  "sftp.navigation": "ディレクトリ移動",
  "sftp.back": "戻る",
  "sftp.forward": "進む",
  "sftp.homeDirectory": "ホームディレクトリ",
  "sftp.rootDirectory": "ルートディレクトリ",
  "sftp.filter": "リモート項目を絞り込み",
  "sftp.filterPlaceholder": "絞り込み",
  "sftp.up": "上へ",
  "sftp.parentDirectory": "親ディレクトリ",
  "sftp.createActions": "作成・アップロード",
  "sftp.noSelectionActions": "項目を選択すると操作できます",
  "sftp.selectedActions": "{name}の操作",
  "sftp.selectedActionsCount": "選択した{count}件の操作",
  "sftp.selected": "選択中：{name}",
  "sftp.selectedCount": "{count}件を選択中",
  "sftp.selectAll": "すべての項目を選択",
  "sftp.selectEntry": "{name}を選択",
  "sftp.clearSelection": "選択を解除",
  "sftp.invertSelection": "選択を反転",
  "sftp.copyName": "名前をコピー",
  "sftp.copyNames": "名前をまとめてコピー",
  "sftp.copyPath": "フルパスをコピー",
  "sftp.copyPaths": "フルパスをまとめてコピー",
  "sftp.openFolder": "フォルダを開く",
  "sftp.editFile": "ファイルを編集",
  "sftp.newFolder": "新規フォルダ",
  "sftp.upload": "アップロード",
  "sftp.uploadFolder": "フォルダをアップロード",
  "sftp.dropHint": "またはファイル・フォルダをドロップ",
  "sftp.dropNow": "ドロップしてフォルダごとアップロード",
  "sftp.dropZone":
    "現在のリモートディレクトリへファイル・フォルダをアップロード",
  "sftp.uploads": "ファイルのアップロード",
  "sftp.upload.pending": "準備中",
  "sftp.upload.queued": "待機中",
  "sftp.upload.uploading": "アップロード中…",
  "sftp.upload.paused": "一時停止",
  "sftp.upload.reattach": "同じファイルを選択して再開",
  "sftp.upload.needs_overwrite": "上書き確認待ち",
  "sftp.upload.done": "完了",
  "sftp.upload.failed": "失敗",
  "sftp.upload.skipped": "スキップ",
  "sftp.upload.cancelled": "キャンセル済み",
  "sftp.cancelTransfer": "転送をキャンセル",
  "sftp.activeTransfers": "転送中 {count}",
  "sftp.transfer.pause": "一時停止",
  "sftp.transfer.resume": "再開",
  "sftp.transfer.clear": "完了分を消去",
  "sftp.manager.heading": "転送マネージャー",
  "sftp.manager.limit": "同時転送は最大 {count} 件",
  "sftp.manager.actions": "転送キューの操作",
  "sftp.manager.collapse": "転送マネージャーを折りたたむ",
  "sftp.manager.expand": "転送マネージャーを展開",
  "sftp.manager.pauseAll": "すべて一時停止",
  "sftp.manager.resumeAll": "すべて再開",
  "sftp.manager.cancelAll": "すべてキャンセル",
  "sftp.manager.items": "転送ファイル",
  "sftp.manager.file": "ファイル",
  "sftp.manager.folder": "フォルダ",
  "sftp.manager.upload": "アップロード",
  "sftp.manager.download": "ダウンロード",
  "sftp.manager.remaining": "残り{duration}",
  "sftp.manager.retry": "再実行",
  "sftp.manager.retryFailed": "失敗した {count} 件を再実行",
  "sftp.manager.status.queued": "待機中",
  "sftp.manager.status.running": "転送中…",
  "sftp.manager.status.paused": "一時停止",
  "sftp.manager.status.reattach": "同じファイルを選択",
  "sftp.manager.status.needs_overwrite": "上書き確認待ち",
  "sftp.manager.status.completed": "完了",
  "sftp.manager.status.failed": "失敗",
  "sftp.manager.status.cancelled": "キャンセル済み",
  "sftp.notice.heading": "転送通知",
  "sftp.notice.completed": "{direction}完了：{name}",
  "sftp.notice.failed": "{direction}失敗：{name}（{problem}）",
  "sftp.notice.dismiss": "通知を閉じる",
  "sftp.download": "ダウンロード",
  "sftp.downloads": "ダウンロード",
  "sftp.download.downloading": "ダウンロード中…",
  "sftp.download.done": "完了",
  "sftp.download.failed": "失敗",
  "sftp.download.cancelled": "キャンセル済み",
  "sftp.name": "名前",
  "sftp.size": "サイズ",
  "sftp.type": "種別",
  "sftp.type.file": "ファイル",
  "sftp.type.directory": "フォルダ",
  "sftp.type.symlink": "シンボリックリンク",
  "sftp.type.other": "その他",
  "sftp.modified": "更新日時",
  "sftp.permissions": "権限",
  "sftp.actions": "操作",
  "sftp.entries": "リモート項目",
  "sftp.rename": "名前変更",
  "sftp.delete": "削除",
  "sftp.chmod": "権限変更",
  "sftp.chmodPrompt": "権限（8 進数、例：640）",
  "sftp.chmodInvalid": "000〜777の3桁の8進数で入力してください。",
  "sftp.overwriteHeading": "このリモートファイルを上書きしますか？",
  "sftp.overwrite": "上書き",
  "sftp.skip": "スキップ",
  "sftp.cancel": "キャンセル",
  "sftp.save": "保存",
  "sftp.close": "閉じる",
  "sftp.editorEmpty": "UTF-8のテキストファイルを選ぶと、ここで編集できます。",
  "sftp.editorLoading": "エディタを読み込んでいます…",
  "sftp.unsaved": "未保存",
  "sftp.unsavedBlocked":
    "移動する前に、編集中のファイルを保存または閉じてください。",
  "sftp.conflict":
    "リモートファイルが更新されています。再読込してから保存してください。",
  "sftp.binaryHint": "UTF-8テキストではありません。ダウンロードしてください。",
  "sftp.tooLargeHint":
    "2 MiBを超えるファイルはダウンロードできますが、編集はできません。",
  "sftp.mkdirPrompt": "フォルダ名",
  "sftp.renamePrompt": "新しい名前",
  "sftp.nameRequired": "名前を入力してください。",
  "sftp.nameInvalid": "名前にスラッシュは使用できません。",
  "sftp.renameUnchanged": "現在とは異なる名前を入力してください。",
  "sftp.deleteHeading": "このリモート項目を削除しますか？",
  "sftp.deleteHeadingCount": "選択した{count}件を削除しますか？",
  "sftp.deleteFailedCount": "選択した項目のうち{count}件を削除できませんでした。",
  "workspace.saved": "保存レイアウトを選択",
  "workspace.new": "新しい保存レイアウト",
  "workspace.save": "名前を付けて保存",
  "workspace.reopen": "この配置を開く",
  "workspace.delete": "削除",
  "workspace.detachPane": "ワークスペースから外す",
  "workspace.live": "現在のワークスペース",
  "workspace.mobilePaneSwitcher": "ワークスペースのターミナル",
  "workspace.oneLiveOnly":
    "現在のワークスペースを閉じてから、別のワークスペースを作成してください。",
  "workspace.dock.left": "左に配置",
  "workspace.dock.right": "右に配置",
  "workspace.dock.top": "上に配置",
  "workspace.dock.bottom": "下に配置",
  "workspace.groupCount": "{count} sessions",
  "workspace.rename": "名前変更",
  "workspace.renameLabel": "{name} の新しい名前",
  "workspace.rowMenu": "{name}のワークスペース操作",
  "workspace.expandGroup": "{name}のターミナルを表示",
  "workspace.collapseGroup": "{name}のターミナルを隠す",
  "workspace.resizeSplit": "分割サイズを変更",
  "workspace.focusMode": "{alias} をフォーカス表示",
  "workspace.exitFocusMode": "フォーカス表示を終了",
  "workspace.movePane":
    "{alias} のペインを移動します。移動先のペインを選んでください。",
  "workspace.movePanePicked":
    "{alias} を選択中です。移動先のペインを選んでください。",
  "workspace.reconnecting": "このペインを再接続してください。",
  "workspace.maxPanes":
    "1 つのワークスペースに配置できるのは最大 {count} ペインです。",
  "workspace.namePrompt": "保存レイアウト名",
  "workspace.nameRequired": "保存レイアウト名を入力してください。",
  "workspace.saveConfirm": "レイアウトを保存",
  "workspace.cancel": "キャンセル",
  "workspace.savedLayouts": "保存レイアウト",
  "workspace.actions": "ワークスペース操作",
  "workspace.savedDescription":
    "SSH 接続先、ローカルシェル、ペイン配置を名前付きで保存し、あとで新しいセッションとして開き直せます。1 レイアウトにつき最大 {count} ペインです。",
  "workspace.broadcastCommand": "コマンドを一括送信…",
  "workspace.broadcastHeading": "接続中のターミナルへ送信",
  "workspace.broadcastDescription":
    "コマンドまたはスニペットを確認してから、接続中の各ターミナルの現在の入力先へ送信します。",
  "workspace.commandClose": "コマンド送信を閉じる",
  "workspace.commandSource": "コマンドの選択",
  "workspace.adHocCommand": "直接入力",
  "workspace.savedSnippet": "保存済みスニペット",
  "workspace.command": "コマンド",
  "workspace.chooseSnippet": "スニペットを選択",
  "workspace.useDefault": "既定値を使用",
  "workspace.targetMode": "接続中のターミナル",
  "workspace.targetCount": "対象：{count} 件",
  "workspace.paneNumber": "ペイン {number}",
  "workspace.targetSkipped": "送信しません（{state}）",
  "workspace.executionNotice":
    "各ターミナルの現在の入力先へコマンドとEnterを送信するため、作業ディレクトリ、環境、シェルの状態をそのまま使います。入力途中の文字や実行中のフォアグラウンドプロセスがある場合は、その続きまたは標準入力として扱われることがあります。",
  "workspace.previewHeading": "ターミナルへ送る内容の確認",
  "workspace.sendTargets": "{count} セッションへ送信",
  "workspace.deliveryResults": "送信結果",
  "workspace.deliveryNotice":
    "出力は各ターミナルに表示されます。送信済みはコマンドの完了を意味しません。",
  "workspace.delivered": "送信済み",
  "workspace.deliveryFailed": "送信できませんでした",
  "snippets.heading": "Snippets",
  "snippets.new": "新規",
  "snippets.empty": "保存済みスニペットはありません。",
  "snippets.name": "名前",
  "snippets.description": "説明",
  "snippets.command": "コマンド",
  "snippets.variableHint":
    "{{name}}で変数を指定します。実行前に展開結果を確認できます。",
  "snippets.variableType": "変数の型",
  "snippets.variableType.string": "文字列",
  "snippets.variableType.integer": "整数",
  "snippets.variableType.boolean": "真偽値",
  "snippets.variableType.secret": "シークレット",
  "snippets.variableType.unknown": "不明な型",
  "snippets.value": "値",
  "snippets.save": "保存",
  "snippets.delete": "削除",
  "snippets.targets": "対象ホスト",
  "snippets.preview": "実行内容を確認",
  "snippets.confirm": "実際のコマンドを確認",
  "snippets.run": "このホストで実行",
  "snippets.results": "実行結果",
  "snippets.status.running": "実行中",
  "snippets.status.completed": "完了",
  "snippets.status.cancelled": "キャンセル済み",
  "snippets.status.queued": "待機中",
  "snippets.status.succeeded": "成功",
  "snippets.status.failed": "失敗",
  "snippets.status.unknown": "状態を確認できません",
  "snippets.cancel": "中止",
  "snippets.startup": "接続時の自動実行",
  "snippets.startupHint":
    "選択したホストのシェル準備後に実行します。シークレット変数は使用できません。",
  "snippets.setStartup": "接続時に自動実行",
  "snippets.clearStartup": "解除",
  "host.duplicateKeyword":
    "このブロックの前の行が同じキーワードを使っています。OpenSSH は最初の 1 つを採用します。",

  "terminal.consoleList": "開いているセッション",
  "terminal.noSessions": "開いているセッションはありません。",
  "terminal.openShell": "Local shell",
  "terminal.openShellOnce": "別の Local shell を今回だけ開く",
  "terminal.rowDetail": "{status} · {destination}",
  "terminal.running": "接続中",
  "terminal.connecting": "接続を開始中",
  "terminal.connected": "接続済み",
  "terminal.agentWorking": "作業中",
  "terminal.agentAttention": "入力待ち",
  "terminal.agentReady": "完了",
  "terminal.agentUnknown": "状態不明",
  "terminal.agentNotificationAttention": "{subject}が入力を待っています",
  "terminal.agentNotificationCompleted": "{subject}が完了しました",
  "terminal.unreadAttention": "未読：入力待ち",
  "terminal.unreadCompleted": "未読：完了",
  "terminal.unreadWorkspace": "このワークスペースに未読のAgent通知があります",
  "terminal.browserNotificationsHeading": "通知",
  "terminal.browserNotificationsDefault":
    "このタブがバックグラウンドにあるとき、Coding Agent の完了や入力待ちを sshc から通知できます。",
  "terminal.browserNotificationsGranted":
    "ブラウザ通知は許可されています。このタブがバックグラウンドにあるときだけ通知します。",
  "terminal.browserNotificationsDenied":
    "sshc の通知がブロックされています。Coding Agent の通知を使うには、ブラウザのサイト設定で通知を許可してください。",
  "terminal.browserNotificationsUnsupported":
    "このブラウザはWeb通知に対応していません。",
  "terminal.browserNotificationsEnable": "通知を有効にする",
  "terminal.browserNotificationsTest": "テスト通知を送る",
  "terminal.browserNotificationsEnabled": "通知は有効です",
  "terminal.browserNotificationsReady": "Coding Agent の通知を利用できます。",
  "terminal.browserNotificationsRequestFailed":
    "通知の許可をリクエストできませんでした。",
  "terminal.browserNotificationsDeliveryFailed":
    "通知は許可されていますが、ブラウザで表示できませんでした。",
  "terminal.notificationAttentionSound": "入力待ちの通知音",
  "terminal.notificationCompletedSound": "完了の通知音",
  "terminal.notificationSoundHint": "このブラウザにだけ保存されます。",
  "terminal.notificationSound.none": "音なし",
  "terminal.notificationSound.gentle": "やさしい音",
  "terminal.notificationSound.bell": "ベル",
  "terminal.notificationSound.pulse": "パルス",
  "terminal.notificationPreview": "再生",
  "terminal.notificationPreviewAttention": "入力待ちの通知音を再生",
  "terminal.notificationPreviewCompleted": "完了の通知音を再生",
  "terminal.notificationVolume": "通知音量",
  "terminal.notificationVolumeHint":
    "{volume}% · 両方のAgent通知音に適用されます",
  "terminal.quickCommands": "クイックコマンド",
  "terminal.quickCommandsClose": "クイックコマンドを閉じる",
  "terminal.quickCommandInsert": "ペインに挿入",
  "terminal.quickCommandRun": "実行",
  "terminal.quickCommandCopy": "コピー",
  "terminal.quickCommandSaveSelection": "選択範囲をスニペットとして保存",
  "terminal.quickCommandName": "スニペット名",
  "terminal.quickCommandSave": "スニペットを保存",
  "terminal.quickCommandSaved": "スニペットを保存しました。",
  "terminal.quickCommandContextWarning":
    "挿入と実行は、ペインの現在の入力先へ文字を送ります。入力途中の文字列や、パスワード／パスフレーズなどを求めるフォアグラウンドの入力待ちへ続けて送られる場合があります。",
  "terminal.quickCommandChanged":
    "スニペットまたはペインが変更されました。更新後のプレビューを確認してから続けてください。",
  "terminal.quickCommandInsertUnsafe":
    "改行または制御文字を含むコマンドは、安全に挿入できません。実行またはコピーを使用してください。",
  "terminal.linkActions": "ターミナルリンクの操作",
  "terminal.linkOpenBrowser": "ブラウザで開く",
  "terminal.linkBrowseSFTP": "SFTPで表示",
  "terminal.linkEditSFTP": "SFTPで編集",
  "terminal.linkDownloadSFTP": "SFTPでダウンロード",
  "terminal.linkCopy": "リンクをコピー",
  "sftp.linkTargetInvalid": "このターミナルリンクは利用できなくなりました。",
  "sftp.linkTargetNotFound": "リモートパスが見つかりません。",
  "sftp.linkTargetNotFile": "リモートパスはファイルではありません。",
  "terminal.agentResumeAvailable": "{agent} のセッションを再開できます。",
  "terminal.agentResumeSamePane": "このペインで再開",
  "terminal.agentResumeNewPane": "新しいペインで再開",
  "terminal.agentResumeFailed":
    "Coding Agent のセッションを再開できませんでした。",
  "terminal.agentResumeStale":
    "Coding Agent セッションの状態が変わりました。確認してからもう一度お試しください。",
  "terminal.agentResumeSamePaneBusy":
    "このシェルにはすでに入力があります。新しいペインで再開してください。",
  "terminal.agentResumeUnavailable":
    "この Coding Agent セッションは再開できません。",
  "terminal.agentResumeIdentityChanged":
    "この alias の SSH 接続先が変わったため、Coding Agent セッションを再開しませんでした。",
  "terminal.progressDialing": "{target} へ接続中 · {position}",
  "terminal.progressHostKey": "{target} のホスト鍵を確認中 · {position}",
  "terminal.progressAuthenticating": "{target} で認証中 · {position}",
  "terminal.progressAuthenticated": "{target} の認証完了 · {position}",
  "terminal.progressOpeningSession": "セッションを開始中 · {position}",
  "terminal.reconnectingAttempt": "再接続中（{attempt}/{limit}）",
  "terminal.exitedWith": "終了 {code}",
  "terminal.localhost": "localhost",
  "terminal.emptyHeading": "開いているセッションがありません",
  "terminal.emptyHint":
    "左の一覧から開くか、ホストの「接続」を押してください。",
  "terminal.forwardLocal": "{listen} → {to} を転送中",
  "terminal.forwardDynamic": "{listen} で SOCKS5 プロキシ",
  "terminal.forwardAgent": "ssh-agent をリモートへ転送中",
  "terminal.rowMenu": "{title} の操作",
  "terminal.rename": "名前を変更",
  "terminal.unpinTitle": "自動の名前に戻す",
  "terminal.renameLabel": "{title} の新しい名前",
  "terminal.renameFailed": "セッション名を変更できませんでした。",
  "terminal.duplicate": "この接続を複製",
  "terminal.moveUp": "上へ移動",
  "terminal.moveDown": "下へ移動",
  "terminal.closeSession": "{title} を閉じる",
  "terminal.closeHeading": "{title} を閉じますか？",
  "terminal.closeBody":
    "接続を終了します。実行中のプロセスと表示中の出力は失われます。",
  "terminal.closeForwards":
    "このセッションのポート転送 {count} 件も終了します。",
  "terminal.closeConfirm": "閉じる",
  "terminal.closeCancel": "開いたままにする",
  "desktop.closeAllHeading2": "開いているセッション {count} 件を閉じますか？",
  "desktop.closeAllBody":
    "すべての接続を終了します。実行中のプロセスと表示中の出力は失われます。",
  "desktop.closeAllConfirm": "すべて閉じる",
  "desktop.closeAllCancel": "開いたままにする",
  "terminal.limitReached":
    "同時に開けるセッション数の上限（{max}）に達しました。どれかを閉じてください。",
  "terminal.limitRefused":
    "これ以上セッションを開けません。どれかを閉じてください。",
  "terminal.unresolvable":
    "この接続の設定を解決できませんでした。理由は「Analysis」で確認できます。",
  "terminal.proxyCommandWithJump":
    "ProxyCommand と ProxyJump は同時に指定できません。どちらか一方を削除してください。OpenSSH も同じ設定を拒否します。",
  "terminal.jumpDepthExceeded": "ProxyJump の階層が上限を超えています。",
  "terminal.hostKeyUnknown":
    "ホスト鍵をまだ信頼していません。内容を確認してから対話接続してください。",
  "terminal.hostKeyChanged":
    "ホスト鍵が変わりました。Known Hostsを確認してから接続し直してください。",
  "terminal.hostKeyRevoked":
    "ホスト鍵は失効済みとして登録されているため接続できません。",
  "terminal.identityUnavailable":
    "利用できる秘密鍵または ssh-agent の鍵がありません。",
  "terminal.authenticationUnavailable":
    "この接続で利用できる認証方式がありません。",
  "terminal.authenticationCancelled": "認証を中止しました。",
  "terminal.keyPassphraseRequired":
    "秘密鍵のパスフレーズが必要です。保存済み認証情報を解除または更新してください。",
  "terminal.reconnectFailed":
    "再接続に失敗しました。設定された上限まで再試行します。",
  "terminal.reconnectExhausted":
    "再接続の上限に達しました。ネットワーク復旧後に新しい接続を開いてください。",
  "terminal.manualReconnect": "再接続",
  "terminal.manualReconnecting": "接続中…",
  "terminal.manualReconnectFailed":
    "SSH セッションへ再接続できませんでした。接続設定とネットワークを確認して、もう一度試してください。",
  "terminal.openFailed": "ターミナルを開けませんでした。",
  "terminal.keyBar": "画面上のキー",
  "terminal.closeFailed": "セッションを閉じられませんでした。",
  "terminal.linkConnecting": "接続しています…",
  "terminal.linkRetrying": "接続しています…（{attempt} 回目）",
  "terminal.linkWaiting":
    "接続が切れました。{seconds} 秒後に {attempt} 回目の再接続を試みます。",
  "terminal.linkStopped":
    "再試行を停止しました。セッションは残っているため、いつでも再接続できます。",
  "terminal.linkGone": "セッションが存在しないため、再接続できません。",
  "terminal.linkNow": "今すぐ接続",
  "terminal.linkStop": "再試行をやめる",
  "terminal.clipboardRefused": "クリップボードにアクセスできませんでした。",
  "terminal.search": "検索",
  "terminal.searchInput": "ターミナル出力を検索",
  "terminal.searchPlaceholder": "スクロールバックを検索…",
  "terminal.searchNoResults": "該当なし",
  "terminal.searchPrevious": "前の一致",
  "terminal.searchNext": "次の一致",
  "terminal.searchClose": "検索を閉じる",
  "terminal.searchCaseSensitive": "大文字と小文字を区別",
  "terminal.searchRegex": "正規表現を使う",
  "terminal.searchInvalidRegex": "不正な式",
  "terminal.copyContext": "直近のターミナル出力をコピー",
  "terminal.copyContextHint":
    "制御シーケンスを除き、直近最大 200 行をコピーします",
  "terminal.copyContextDone": "直近のターミナル出力をコピーしました。",
  "terminal.copyContextEmpty": "コピーできるターミナル出力がありません。",
  "terminal.osc52Hint":
    "このターミナルセッションから OSC 52 でシステムのクリップボードへ書き込むことを許可します",
  "terminal.osc52Enabled":
    "このターミナルセッションで OSC 52 のクリップボード書き込みを許可しました。",
  "terminal.osc52Disabled": "OSC 52 のクリップボード書き込みを拒否します。",
  "terminal.osc52Copied":
    "リモートアプリがシステムのクリップボードへコピーしました。",
  "terminal.moreActions": "ターミナルのその他の操作",
  "terminal.portForwarding": "ポート転送",
  "terminal.forwardDescription":
    "{title} の接続中の SSH を使うトンネルを管理します。",
  "terminal.forwardClose": "閉じる",
  "terminal.forwardActive": "動作中の転送",
  "terminal.forwardNone": "この接続で動作中の転送はありません。",
  "terminal.forwardTemporary": "このセッションのみ",
  "terminal.forwardSaved": "保存済み",
  "terminal.forwardSavedStopHint":
    "ここで停止しても接続設定からは削除されません。",
  "terminal.forwardRetryHint":
    "ローカルポートの競合を解消し、このSSHを再接続すると保存済み転送を再試行します。",
  "terminal.forwardAgentLabel": "SSH エージェント",
  "terminal.forwardCopy": "アドレスをコピー",
  "terminal.forwardCopied": "アドレスをコピーしました。",
  "terminal.forwardCopyFailed": "アドレスをコピーできませんでした。",
  "terminal.forwardStop": "停止",
  "terminal.forwardStopping": "停止中…",
  "terminal.forwardStopped": "転送を停止しました。",
  "terminal.forwardNew": "転送を開始",
  "terminal.forwardSaveConnection": "この接続設定にも保存する",
  "terminal.forwardSaveHint":
    "今すぐ開始し、次回この接続を開いたときにも復元します。",
  "terminal.forwardSaveUnavailable":
    "ローカルシェルまたは未保存の接続には設定を保存できません。",
  "terminal.forwardNeedsConnection":
    "SSHを再接続してから転送を開始してください。",
  "terminal.forwardStart": "開始",
  "terminal.forwardStarting": "開始中…",
  "terminal.forwardStarted": "このセッションで転送を開始しました。",
  "terminal.forwardStartedAndSaved": "転送を開始し、接続設定にも保存しました。",
  "terminal.forwardPausedReconnect": "SSHの再接続中は転送を利用できません。",
  "terminal.forwardBindFailed": "ローカルポートを開けませんでした: {detail}",
  "terminal.forwardUnavailable":
    "現在のSSH接続ではポート転送を変更できません。",
  "terminal.forwardInvalid": "転送の種類、ポート、転送先を確認してください。",
  "terminal.forwardSaveFailed":
    "転送は開始しましたが、接続設定へ保存できませんでした。",
  "terminal.forwardFailed": "ポート転送の操作を完了できませんでした。",
  "terminal.settingsHeading": "ターミナル",
  "terminal.settingsSaved":
    "保存しました。クリップボード設定はすぐに反映され、その他はこれから開くターミナルが使います。",
  "terminal.settingsLoading": "ターミナル設定を読み込んでいます…",
  "terminal.settingsStorageHint":
    "ターミナルの外観を含むこれらの設定はworkspace metadataに保存され、バックアップと同期の対象になります。テーマ、言語、通知音はこのブラウザにだけ保存されます。",
  "terminal.maxSessionsLabel": "最大セッション数",
  "terminal.maxSessionsHint":
    "1〜200。空欄の場合は 50 です。上限に達すると新しいセッションを開けません。既存のセッションは自動的に閉じられません。",
  "terminal.scrollbackLabel": "Engine再生バッファ（バイト）",
  "terminal.scrollbackHint":
    "ブラウザ再接続時にengineが再生する出力です。16384〜4194304、空欄は262144（256 KiB）。メモリだけに保持し、ディスクへ書きません。",
  "terminal.browserScrollbackLabel": "ブラウザのスクロールバック（行）",
  "terminal.browserScrollbackHint":
    "各ブラウザ端末が保持する行数です。1000〜100000、空欄は5000。増やすほどブラウザのメモリを使用します。",
  "terminal.localShellProfileLabel": "既定のローカルシェル",
  "terminal.localShellProfileHint":
    "このマシンで検出・検証できた実行ファイルから選びます。ローカル端末を開くときだけ別のシェルを選ぶこともできます。",
  "terminal.localShellProfileSystem": "システムのログインシェル",
  "terminal.osc52DefaultLabel": "OSC 52のクリップボード書き込みを既定で許可",
  "terminal.osc52DefaultHint":
    "ローカルシェルはこの設定を使います。SSH接続ごとに継承・許可・拒否を選べます。",
  "terminal.jisYenBackslashLabel": "JIS配列の¥キーをbackslashとして送る",
  "terminal.jisYenBackslashHint":
    "日本語キーボード向けです。IME変換中の入力は変更しません。",
  "terminal.fontSizeLabel": "文字の大きさ",
  "terminal.paletteLabel": "配色",
  "terminal.verbosityLabel": "接続のログ",
  "terminal.verbosityHint":
    "ssh -v と同等の接続情報をターミナルへ表示します。新しい接続から適用されます。",
  "terminal.verbosityQuiet": "表示しない",
  "terminal.verbosityBrief": "基本情報（-v）",
  "terminal.verbosityDetailed": "鍵・経由地・所要時間（-vv）",
  "terminal.verbosityFull": "すべて（-vvv）",
  "terminal.reconnectLabel": "接続が切れたときの再接続",
  "terminal.reconnectHint":
    "予期せず切断された場合の再接続回数です。1、2、5、10、15 秒を基準に再試行を分散するため、5 回の場合は最大 40 秒かかります。手動で閉じたセッションは再接続しません。",
  "terminal.reconnectDefault": "既定（5 回・最大 40 秒）",
  "terminal.reconnectNever": "再接続しない",
  "terminal.reconnectOnce": "1 回（最大 2 秒）",
  "terminal.reconnectTwice": "2 回（最大 4 秒）",
  "terminal.reconnectThrice": "3 回（最大 10 秒）",
  "terminal.reconnectFive": "5 回（最大 40 秒）",
  "engine.heading": "エンジン",
  "engine.portLabel": "ポート",
  "engine.portHint":
    "空欄の場合、デスクトップでは端末固有の固定ポート（初期値54447）を使用します。ブックマークやインストールしたWebアプリを継続して使えます。次回のエンジン起動時から適用されます。",
  "engine.portOutOfRange": "ポートは 1024〜65535 の範囲で指定してください。",
  "engine.loading": "エンジン設定を読み込んでいます…",
  "engine.saved": "保存しました。自動ロックはすぐに、ポートの変更は次回のエンジン起動時から適用されます。",
  "engine.saveFailed": "保存できませんでした。",
  "engine.vaultAutoLockLabel": "Vault の自動ロック",
  "engine.vaultAutoLockHint":
    "保存済みパスワードや鍵パスフレーズを使用しない時間が続いた場合に Vault をロックします。状態確認、Terminal の出力、バックグラウンド同期では時間を延長しません。",
  "engine.vaultAutoLockIdle": "操作がない場合にロック",
  "engine.vaultAutoLockRestart": "自動ロックしない",
  "engine.vaultAutoLockValue": "時間",
  "engine.vaultAutoLockUnit": "単位",
  "engine.vaultAutoLockMinutes": "分",
  "engine.vaultAutoLockHours": "時間",
  "engine.vaultAutoLockOutOfRange": "自動ロック時間は 1〜999 の整数で指定してください。",
  "engine.vaultAutoLockRestartWarning":
    "Vault は自動でロックされません。手動でロックしない限り、sshc を再起動するまでロック解除状態が続きます。自分で管理している端末でのみ使用してください。",
  "terminal.fontLabel": "フォント",
  "terminal.backgroundLabel": "背景画像",
  "terminal.backgroundHint":
    "画像はワークスペースに保存され、バックアップと同期の対象になります。",
  "terminal.backgroundNone": "画像なし",
  "terminal.backgroundFollowsOverall": "全体の設定に従う",
  "terminal.backgroundAdd": "画像を追加",
  "terminal.backgroundRemove": "{name} を削除",
  "terminal.backgroundRoom": "残り {megabytes} MB",
  "terminal.backgroundTooLarge": "この画像は保存できる大きさを超えています。",
  "terminal.backgroundsFull": "これ以上画像を置く余地がありません。",
  "terminal.backgroundNotAnImage":
    "このファイルは、表示できる画像ではありません。",
  "terminal.backgroundFailed": "画像を保存できませんでした。",
  "terminal.tintLabel": "画像に重ねる色の不透明度",
  "terminal.tintHint":
    "値を大きくすると画像が暗くなり、文字を読みやすくできます。",
  "connection.backgroundLabel": "ターミナルの背景画像",
  "connection.backgroundHint": "この接続のターミナルにだけ適用されます。",
  "terminal.fontHint": "JetBrains Mono はアプリに同梱されています。",
  "terminal.fontFollowsSystem": "システムの等幅フォントを使用",
  "terminal.fontFollowsOverall": "全体の設定に従う",
  "connection.fontLabel": "ターミナルのフォント",
  "connection.fontHint": "この接続のターミナルにだけ適用されます。",
  "terminal.paletteHint":
    "個別の配色を指定していないターミナルに適用されます。",
  "terminal.paletteFollowsTheme": "アプリのテーマに従う",
  "terminal.paletteFollowsOverall": "全体の設定に従う",
  "connection.paletteLabel": "ターミナルの配色",
  "connection.paletteHint": "この接続のターミナルにだけ適用されます。",
  "connection.encodingLabel": "接続先の文字コード",
  "connection.encodingHint":
    "ブラウザのターミナルとコマンドラインの sshc の両方で、この接続に使用します。",
  "connection.encodingUTF8": "UTF-8（既定）",
  "connection.encodingShiftJIS": "Shift_JIS（日本語）",
  "connection.encodingEUCJP": "EUC-JP（日本語）",
  "connection.encodingISO2022JP": "ISO-2022-JP（日本語）",
  "connection.osc52Label": "OSC 52クリップボード",
  "connection.osc52Hint":
    "このSSH接続上のremote applicationによるシステムクリップボードへの書き込みを選びます。",
  "connection.osc52Inherit": "Terminal全体の設定を使う",
  "connection.osc52Allow": "この接続では許可",
  "connection.osc52Deny": "この接続では拒否",
  "terminal.fontSizeHint":
    "ピクセル単位で指定します。空欄の場合は、狭い画面で 15、それ以外で 13 になります。",
  "terminal.copyOnSelectLabel": "選択した文字列を自動的にコピーする",
  "terminal.copyOnSelectHint":
    "選択を終えたときに一度だけコピーします。選択によってシステムのクリップボードを置き換えたくない場合はオフにします。",
  "terminal.rightClickPasteLabel": "右クリックで貼り付ける",
  "terminal.rightClickPasteHint":
    "対応する端末では bracketed paste を使います。通常のコンテキストメニューを残す場合はオフにします。",
  "terminal.limitsOutOfRange": "指定値が有効範囲外です。",
  "terminal.startLabel": "開始ディレクトリ",
  "terminal.startHint":
    "ローカルシェルを開始するディレクトリです。~/work または絶対パスで指定してください。~ は展開せずに保存されるため、別のマシンでも同じ指定を使用できます。空欄の場合はホームディレクトリから開始します。",
  "terminal.startSave": "保存",
  "terminal.startMissing": "そのディレクトリはありません。",
  "terminal.startNotADirectory": "それはディレクトリではありません。",
  "terminal.startUnusable":
    "~/から始まるパス、または絶対パスを指定してください。",
  "terminal.settingsSaveFailed": "ターミナル設定を保存できませんでした。",
  "terminal.screenLabel": "{title} のターミナル",
  "terminal.exitedWithCode":
    "プログラムが終了コード {code} で終了しました。出力は、このセッションを閉じるまで表示されます。",
  "terminal.exitedWithSignal":
    "プログラムがシグナル {signal} で終了しました。出力は、このセッションを閉じるまで表示されます。",
  "inspector.appOnly": "sshc 固有の設定",
  "inspector.groupLabel": "グループの表示設定",
  "inspector.hostSavesImmediately": "ここでの変更はすぐに保存されます。",
  "inspector.groupChangesStaged":
    "ここでの変更は「グループを保存」を選ぶまで未保存です。",
  "inspector.notices": "注意",
  "inspector.inherited": "継承した値",
  "inspector.noNotices": "この接続に関する注意事項はありません。",
  "inspector.noInherited":
    "この接続の値はすべて同じ Host ブロック内で定義されています。",
  "shell.language": "Lang",
  "shell.languageMenu": "言語メニュー",
  "shell.languageEnglish": "English",
  "shell.languageJapanese": "日本語",
  "shell.theme": "外観",
  "shell.themeMenu": "テーマメニュー",
  "shell.themeSystem": "システムに合わせる",
  "shell.themeLight": "ライト",
  "shell.themeDark": "ダーク",

  "section.home": "Home",
  "section.menu": "Menu",
  "section.connections": "Connections",
  "section.terminal": "Terminal",
  "section.config": "SSH Config",
  "section.groups": "Groups",
  "section.keys": "SSH Keys",
  "section.knownHosts": "Known Hosts",
  "section.remoteKeys": "Remote Keys",
  "section.diagnostics": "Diagnostics",
  "section.settings": "Settings",
  "settings.heading": "Settings",
  "settings.pageDescription":
    "ターミナルの動作、sshc の起動設定、このマシンに保存するデータの暗号化を設定します。",
  "settings.engineDescription":
    "sshc エンジンの待受ポートと Vault の自動ロックを設定します。",
  "settings.terminalDescription":
    "新しいターミナルに適用する動作、表示、操作方法を設定します。",
  "settings.notificationsDescription":
    "ブラウザ通知と、エージェントの状態変化を知らせる音を設定します。",
  "settings.connectionsDescription":
    "このブラウザで開いている接続を確認し、まとめて終了します。",
  "settings.passwordDescription":
    "このマシンの暗号化データを保護するマスターパスワードを変更します。",
  "secrets.heading": "Vault",
  "secrets.pageDescription":
    "名前付きのアカウントパスワードと鍵パスフレーズを管理します。値は編集時だけ表示されます。",
  "secrets.metricPasswords": "アカウントパスワード",
  "secrets.metricPassphrases": "鍵パスフレーズ",
  "secrets.metricAssignments": "割り当て",
  "secrets.loading": "Vault を読み込んでいます…",
  "secrets.explainNew":
    "1 つのマスターパスワードで、アカウントのパスワード、鍵のパスフレーズ、オブジェクトストレージの認証情報、Snippet を暗号化します。マスターパスワードは保存されず、復旧もできません。紛失すると、これらの暗号化データを復号できなくなります。OpenSSH が直接読み込むファイルには影響しません。",
  "secrets.explainLocked":
    "Vault はロックされています。アクセスするにはマスターパスワードを入力してください。",
  "secrets.master": "マスターパスワード",
  "secrets.create": "Vault を作成",
  "secrets.unlock": "開く",
  "secrets.lock": "sshc をロック",
  "secrets.createFailed":
    "Vault を作成できませんでした。マスターパスワードは 12 文字以上必要です。",
  "secrets.unlockFailed": "マスターパスワードが違います。",
  "secrets.failed": "Vault を読み込めませんでした。",
  "secrets.storeFailed": "保存できませんでした。",
  "secrets.deleteFailed": "削除できませんでした。",
  "secrets.inUse":
    "この認証情報は使用中です。削除する前に割り当て先を変更してください。",
  "secrets.none": "まだ何も保存されていません。",
  "secrets.assignedHosts": "割り当て先のホスト",
  "secrets.noAssignedHosts": "割り当て先のホストはありません",
  "secrets.keys": "対象の鍵",
  "secrets.noKeys": "対象の鍵はありません",
  "secrets.dedicated": "この鍵専用",
  "secrets.removeDedicated": "{key} の保存済みパスフレーズを削除",
  "secrets.keyHostUsageIncomplete":
    "SSH 設定をすべて読み込めなかったため、この鍵を使用しているホストを完全には確認できませんでした。SSH Config の診断を確認してください。",
  "secrets.keyHostsUnavailable": "割り当て先ホストを確認できません",
  "secrets.delete": "{name} を削除",
  "secrets.edit": "{name} を編集",
  "secrets.editPassword": "アカウントパスワードを編集",
  "secrets.editPassphrase": "鍵パスフレーズを編集",
  "secrets.editNote":
    "保存済みの値を表示しています。名前を変更しても、現在の割り当て先は維持されます。",
  "secrets.credentialName": "名前",
  "secrets.passwordValue": "パスワード",
  "secrets.passphraseValue": "鍵パスフレーズ",
  "secrets.revealing": "保存済みの値を読み込んでいます…",
  "secrets.revealFailed": "保存済みの値を表示できませんでした。",
  "secrets.updateFailed": "変更を保存できませんでした。",
  "secrets.nameExists":
    "同じ名前の認証情報が既にあります。別の名前を指定してください。",
  "secrets.saveChanges": "変更を保存",
  "secrets.saving": "保存中…",
  "secrets.cancel": "キャンセル",
  "secrets.passwordsHeading": "アカウントのパスワード",
  "secrets.passphrasesHeading": "鍵のパスフレーズ",
  "secrets.newPasswordName": "新しいアカウントパスワードの名前",
  "secrets.newPasswordValue": "新しいアカウントパスワードの値",
  "secrets.storePassword": "アカウントパスワードを保存",
  "secrets.newPassphraseName": "新しい鍵パスフレーズの名前",
  "secrets.newPassphraseValue": "新しい鍵パスフレーズの値",
  "secrets.storePassphrase": "鍵パスフレーズを保存",
  "update.version": "バージョン {version}",
  "update.available": "{version} が公開されています — 変更点を読む",
  "desktop.closeAllHeading": "開いている接続",
  "desktop.closeAllNote":
    "すべてのセッション、ポート転送、ssh-agent 転送を終了します。エンジンは動作を続けます。",
  "desktop.openCount": "{count} 件",
  "desktop.closeAll": "接続をすべて閉じる",
  "secrets.changeHeading": "マスターパスワード",
  "secrets.changeNote":
    "変更すると、ローカルの Vault、Snippet、同期設定、このマシンの全バックアップを新しいパスワードで再暗号化します。リモートスナップショットは別の同期鍵を使うため書き換えません。",
  "secrets.currentMaster": "現在のマスターパスワード",
  "secrets.newMaster": "新しいマスターパスワード",
  "secrets.confirmMaster": "新しいマスターパスワード（確認）",
  "secrets.change": "マスターパスワードを変更",
  "secrets.wrongCurrent":
    "現在のマスターパスワードが違います。何も変更していません。",
  "secrets.changeFailed": "マスターパスワードを変更できませんでした。",
  "secrets.changedMasterLocally":
    "マスターパスワードを変更しました。ローカルの Vault、Snippet、同期設定、バックアップは新しいパスワードで暗号化されています。リモートスナップショットは書き換えていません。",
  "section.secrets": "Vault",
  "lock.explainNew":
    "マスターパスワードを設定してください。保存済みパスワード、鍵のパスフレーズ、Snippet、同期設定、sshc が作成するすべてのバックアップを暗号化します。",
  "lock.explainOpen": "sshc を開くにはマスターパスワードを入力してください。",
  "lock.noRecovery":
    "マスターパスワードは復旧できません。紛失すると Vault、Snippet、暗号化されたバックアップを開けなくなります。",
  "lock.password": "マスターパスワード",
  "lock.confirm": "マスターパスワード（確認）",
  "lock.create": "Vault を作成",
  "lock.open": "開く",
  "lock.wrong": "マスターパスワードが違います。",
  "lock.tooShort": "マスターパスワードは {count} 文字以上必要です。",
  "lock.alreadyExists":
    "アプリ内に Vault が見つかりました。マスターパスワードを入力して開いてください。",
  "lock.storagePermission":
    "Android がアプリ専用ストレージへのアクセスを拒否しました。下の詳細をコピーして共有してください。",
  "lock.storageFull":
    "空き容量が不足しているため Vault を作成または更新できません。",
  "lock.storageReadOnly":
    "アプリ専用ストレージが読み取り専用です。端末を再起動してから再試行してください。",
  "lock.storageBusy":
    "別の Vault 更新が完了していません。少し待ってから再試行してください。",
  "lock.storageIO":
    "Android がアプリ専用ストレージへの入出力エラーを報告しました。",
  "lock.schemaOlder":
    "Vault のバージョンが古いです（必要なバージョン：{required}、現在：{current}）。",
  "lock.schemaNewer":
    "Vault のバージョンがこの sshc より新しいです（対応バージョン：{required}、現在：{current}）。",
  "lock.migrationFailed":
    "Vault をバージョン {current} から {required} へ更新できませんでした。元の Vault は変更していません。",
  "lock.migrationCompleted":
    "Vault をバージョン {current} から {required} へ安全に更新しました。",
  "lock.migrationDismiss": "更新通知を閉じる",
  "lock.envelopeUnsupported":
    "この形式の暗号化 Vault には対応していません。診断情報を開いて共有してください。",
  "lock.schemaRecoveryHint":
    "まず互換性のある最新のローカルバックアップを復元できます。見つからない場合は、SSH 設定と鍵ファイルを残して空の Vault を作成できます。",
  "lock.restoreCompatibleBackup": "互換性のある Vault を復元",
  "lock.noCompatibleBackup":
    "互換性のあるローカルバックアップは見つかりませんでした。何も変更していません。",
  "lock.recoveryFailed": "互換性のある Vault を復元できませんでした。",
  "lock.resetUnsupportedAcknowledge":
    "保存済みパスワード、保存済み鍵パスフレーズ、同期設定が初期化されることを確認しました。SSH 設定と鍵ファイルは残ります。",
  "lock.resetUnsupported": "空の Vault を作成",
  "lock.resetFailed": "未対応の Vault を安全に置き換えられませんでした。",
  "lock.failed": "Vault を開けませんでした。",
  "section.sync": "Sync",
  "section.history": "History",

  "home.heading": "Connections",
  "home.manageConnections": "Connections を管理",
  "home.connections": "Connections",
  "home.groups": "グループ",
  "home.attention": "確認が必要",
  "home.quickConnect": "Quick Connect",
  "home.quickConnectHint":
    "最近接続した順に表示し、未接続の接続先は名前順に並べます。",
  "home.recentConnections": "最近使った接続",
  "home.recentConnectionsHint": "この端末で接続に成功した SSH 接続です。",
  "home.recentConnectionList": "最近使った接続先",
  "home.savedWorkspaces": "保存レイアウト",
  "home.savedWorkspacesHint":
    "保存した接続先とペイン配置を、新しい SSH 接続としてまとめて開きます。",
  "home.savedWorkspaceList": "保存済みターミナルレイアウト",
  "home.workspacePanes": "{count} ペイン",
  "home.workspaceUpdated": "更新 {at}",
  "home.openWorkspace": "レイアウトを開く",
  "home.lastConnected": "最終接続 {at}",
  "home.connectionList": "利用できる接続",
  "home.search": "接続を検索",
  "home.searchPlaceholder": "ホスト、グループ、タグを検索",
  "home.viewMode": "接続先の表示形式",
  "home.panelView": "パネル",
  "home.listView": "リスト",
  "home.groupFilter": "グループで接続先を絞り込み",
  "home.allGroups": "すべて",
  "home.groupCount": "グループ {count} 件",
  "home.connectionCount": "接続先 {count} 件",
  "home.groupBreadcrumb": "選択中のグループ",
  "home.openGroup": "{name} を開く（接続先 {count} 件）",
  "home.noChildGroups": "この階層にグループはありません。",
  "home.pointerHint": "マウス：ダブルクリック · タッチ：1 回タップ",
  "home.touchHint": "1 回タップで接続",
  "home.connectGesture":
    "{alias} へ接続します。マウスではダブルクリック、タッチ画面では 1 回タップします。",
  "home.neverConnected": "接続履歴なし",
  "home.loading": "SSH 設定を読み込んでいます…",
  "home.noConnections":
    "接続先がまだありません。SSH Config に Host を追加してください。",
  "home.noMatches": "検索に一致する接続はありません。",
  "home.groupMissingDetail": "選択中のグループ {name} は、すでに存在しません。",
  "home.ungrouped": "グループなし",
  "home.tagsFor": "{alias} のタグ",
  "home.connectionActions": "{alias} の操作",
  "home.openConnectionSettings": "接続設定を開く",
  "home.connect": "接続",
  "home.opening": "接続中…",
  "home.loadFailed": "SSH 設定を読み込めませんでした。",
  "home.workspace": "ワークスペース",
  "home.workspaceUnavailable": "ワークスペースの状態を取得できません。",
  "home.workspaceClean": "設定の問題や中断した変更はありません。",
  "home.workspaceAttention":
    "設定または復旧について {count} 件の確認が必要です。",
  "home.openConfig": "設定を確認",
  "home.recoverChanges": "変更を復旧",
  "home.sync": "同期",
  "home.syncUnavailable": "同期状態を取得できません。",
  "home.syncNotConfigured": "リモート同期は設定されていません。",
  "home.syncNever": "リモートバケットは設定済みですが、まだ同期していません。",
  "home.syncLast": "最終同期 {at} · {count} ファイル",
  "home.openSync": "同期画面を開く",

  "copy.button": "{label}をコピー",
  "copy.done": "コピーしました。",
  "copy.refused": "ブラウザがクリップボードへの書き込みを拒否しました。",
  "copy.command": "コマンド",
  "copy.terminalCommand": "ターミナルコマンド",
  "copy.privateKey": "秘密鍵",
  "copy.publicKey": "公開鍵",
  "copy.keyLine": "鍵の行",
  "copy.remoteCommand": "リモートコマンド",
  "copy.diagnosticReport": "診断レポート",

  "diagnostic.requestFailed": "操作を完了できませんでした",
  "diagnostic.requestFailedHint":
    "sshc でエラーが発生しました（{code}）。問い合わせの際は診断情報を確認してください。",
  "diagnostic.showDetails": "診断情報を表示",
  "diagnostic.dismiss": "エラーを閉じる",

  "history.requestRejected": "要求が拒否されました（{code}）。",
  "history.pageTitle": "History",
  "history.pageDescription":
    "完了した変更を確認し、中断した書き込みを復旧し、新しい履歴を失わずにファイル単位で復元できます。",
  "history.metricChanges": "完了した変更",
  "history.metricInterrupted": "中断",
  "history.metricRestorable": "復元可能なファイル",
  "history.operation.configuration": "SSH Config の変更",
  "history.operation.connection": "接続先の変更",
  "history.operation.key": "鍵の変更",
  "history.operation.terminal": "ターミナル設定の変更",
  "history.operation.engine": "起動設定の変更",
  "history.operation.vault": "Vault の変更",
  "history.operation.sync": "同期設定の変更",
  "history.operation.other": "アプリの変更",
  "history.status.staging": "準備中",
  "history.status.staged": "中断",
  "history.status.applied": "適用済み",
  "history.status.completed": "完了",
  "history.status.rolledBack": "取り消し済み",
  "history.status.unknown": "記録済み",
  "history.interrupted": "中断した変更",
  "history.interruptedDetail":
    "{operation} 開始 {startedAt}：{total} 個のうち {committed} 個のファイルが書き込まれました。",
  "history.complete": "完了させる",
  "history.rollBack": "取り消す",
  "history.loading": "履歴を読み込んでいます…",
  "history.restored": "{path} を復元しました（履歴 ID：{id}）。",
  "history.completedTransaction": "中断していた変更を完了しました。",
  "history.rolledBack": "中断していた変更を取り消しました。",
  "history.completed": "完了した変更",
  "history.empty": "このアプリケーションを通した変更はまだありません。",
  "history.restorePath": "{path} を復元",
  "history.backupsKept":
    "世代バックアップは ~/.ssh/sshc/backups に保存され、自動では削除されません。復元操作も新しい変更として履歴に残るため、同じ方法で取り消せます。",

  "notice.complex_external_rule":
    "ワイルドカード、否定、Match ブロック、alias の重複のいずれかが含まれるため、この値を単純な形式では編集できません。値の参照元を表示します。",
  "notice.duplicate_alias":
    "別のブロックが同じ alias を宣言しています。OpenSSH は最初に読んだものを使います。",
  "notice.wildcard_shadow":
    "すべてに一致するブロックが、このホストの値に影響する可能性があります。",
  "notice.negated_pattern": "否定パターンがここに適用されます。",
  "notice.unnamed_host_block":
    "このブロックには具体的な alias がないため、Raw テキストでのみ編集できます。",
  "notice.match_block":
    "Match ブロックが見つかりました。Match exec はコマンドを実行しうるため、ここでは評価しません。",
  "notice.dangerous_directive":
    "このディレクティブはコマンドを実行する可能性があります。記述どおりに保存され、このアプリが実行することはありません。",
  "notice.unstructured_line":
    "この行は引用符が対応しておらず、記述されたまま保持されます。",
  "notice.external_file":
    "このファイルは ~/.ssh の外にあります。表示のみで、書き込みは行いません。",
  "notice.orphan_metadata":
    "このメモに対応するホストが存在しません。接続先を確認して関連付け直してください。",
  "notice.group_cycle":
    "このグループの親が循環しているため、スキップしました。",
  "notice.group_member_missing":
    "このグループのメンバーに対応する Host ブロックが設定にありません。",
  "refusal.directory_not_empty":
    "ディレクトリが空ではありません。先に中のファイルを削除してください。ファイルを直接参照する Include 行も同時に更新されます。",
  "refusal.not_a_directory": "そのパスはディレクトリではなくファイルです。",
  "refusal.group_is_declared":
    "{detail} は宣言済みのグループです。グループ画面で名前を変更または削除してください。関連する接続、共通設定、鍵も同時に移動されます。",
  "refusal.destination_exists":
    "移動先には同名のファイルまたはディレクトリがあります。",
  "refusal.alias_already_declared":
    "同名の接続設定が既にあるため保存できません。別の名前を使用してください。",
  "refusal.region_damaged":
    "~/.ssh/config にある sshc 生成ブロックのマーカーが片方しかありません。生成範囲を特定できないため、何も書き込んでいません。",
  "notice.group_not_declared":
    "このディレクトリは connections/ の下にありますが、参照する Include 行がないため読み込まれません。グループとして宣言するか、中のファイルを移動してください。",
  "notice.group_directory_missing":
    "このグループは宣言されていますが、ディレクトリがありません。ディレクトリを作成してファイルを配置するまで、設定は読み込まれません。",
  "notice.group_empty": "このグループは宣言されていますが、中身がありません。",
  "notice.generated_region_damaged":
    "~/.ssh/config に sshc 生成ブロックの開始マーカーはありますが、終了マーカーがありません。sshc が生成した範囲を特定できないため、グループを保存できません。ブロック内の Include 行は現在も有効です。最後の Include 行の次に「# <<< sshc groups」を追加するか、生成ブロック全体を削除してからグループを保存し直してください。",
  "notice.explained_values_only":
    "設定の一部を読み込めなかったため、読み込み可能な範囲の値だけを表示しています。",
  "notice.match_exec_refused":
    "この設定には Match exec があります。ここでは何も実行しないので、値を解決できません。端末から ssh で接続してください。",
  "notice.match_final_refused":
    "この設定には Match final があります。OpenSSH は設定を二度読み込みますが、sshc はこの処理に対応していないため値を解決できません。",
  "notice.canonicalise_refused":
    "この設定は CanonicalizeHostname を有効にしています。設定を読み直す必要があるため、ここでは値を解決できません。",
  "notice.unknown_token_refused":
    "この設定では、sshc が対応していないトークンを使用しています。値を解決できません。",
  "notice.destination_not_included":
    "このファイルは SSH Config から参照されていないため、OpenSSH に読み込まれません。Include 設定を追加してください。",
  "notice.group_file_unreached":
    "このファイルは connections/ 内にありますが、どのグループからも参照されていません。宣言済みのグループへ移動してください。",
  "notice.group_directory_leftover":
    "グループのディレクトリが空になりました。sshc は空のディレクトリを自動削除しません。不要であれば手動で削除してください。",
  "notice.include_no_longer_matches":
    "ファイル名を変更すると現在の Include パターンの対象外になり、OpenSSH に読み込まれなくなります。",
  "notice.include_not_rewritten":
    "sshc が自動更新できない形式の Include がこのファイルを参照しています。Include 設定を手動で確認してください。",
  "notice.include_now_unreached":
    "新しい保存先は SSH Config から参照されていません。Include 設定を追加するまで、OpenSSH に読み込まれません。",

  "preview.heading": "保存内容の確認",
  "preview.newFile": "（新規ファイル）",
  "preview.tooLarge":
    "このファイルは行単位で表示できるサイズを超えているため、ファイル全体の置換として表示します。",
  "preview.syntaxError":
    "{path} の {line} 行 {column} 列に構文エラーがあります。編集はここに保持され、書き込まれていません。",
  "preview.theFile": "対象ファイル",
  "preview.graphError":
    "この変更により Include の参照関係が不正になります。何も書き込んでいません。",
  "preview.conflictError":
    "このファイルはアプリケーションの外で変更されました。何も書き込まれていません。",
  "preview.rejected":
    "要求が拒否されました（{code}）。何も書き込まれていません。",
  "preview.changedOnDisk": "読み込み後にディスク上で変更された内容",
  "preview.pendingChange": "保留中の変更",
  "preview.mergeByHand":
    "ファイルを読み込み直し、2 つの変更を手動で統合してください。何も書き込んでいません。",
  "preview.nothingYet":
    "値を変更すると、何が書き込まれるかがここに表示されます。",
  "preview.explainedFor": "{alias} の解決済み設定値",
  "preview.unset": "未設定",

  "reveal.heading": "秘密鍵を表示：{path}",
  "reveal.warning":
    "秘密鍵はこのページに表示され、このウィンドウを読める人なら誰でもコピーできます。ブラウザ拡張やクリップボード履歴ツールからは、このアプリケーションでは保護できません。表示した事実は履歴に記録されます（鍵そのものは記録しません）。",
  "reveal.show": "秘密鍵を表示",
  "reveal.requesting": "一度限りの確認を要求しています…",
  "reveal.privateKeyLabel": "秘密鍵",
  "reveal.failed":
    "秘密鍵を表示できませんでした。このダイアログを閉じて、もう一度確認してください。",
  "reveal.close": "閉じる",

  "orphan.heading": "接続先が存在しない設定",
  "orphan.explain":
    "これらのメモに対応する Host ブロックは設定から削除されています。関連付け先の接続を指定してください。",
  "orphan.chooseTarget": "このメモが属する接続を選んでください。",
  "orphan.occupied":
    "{alias} には既に独自の設定があります。先にそちらを消すか、このメモを破棄してください。",
  "orphan.entry": "{path} の {alias}",
  "orphan.noSettings": "設定なし",
  "orphan.tags": "タグ {tags}",
  "orphan.note": "メモ「{note}」",
  "orphan.colour": "色 {colour}",
  "orphan.reassociateWith": "{alias} の再関連付け先",
  "orphan.reassociatePlaceholder": "再関連付け先…",
  "orphan.reassociate": "{alias} を再関連付け",
  "orphan.discard": "{alias} の設定を破棄",

  "password.blocker.authenticationOff":
    "このホストは PasswordAuthentication が no のため、クライアントがパスワードを提示することはありません。",
  "password.blocker.aliasNotSimple":
    "これは具体的なホストではなくパターンです。パスワードを保存するには、単一ホストのアカウントを指定してください。",
  "password.blocker.identityFile":
    "このホストには秘密鍵が直接設定されています。sshc はパスワードを保存・供給せず、必要な手入力を OpenSSH に任せます。",
  "password.warn.hostKeyUnknown":
    "このホストの鍵は known_hosts に登録されていません。保存済みパスワードを使用すると、パスワード入力ヘルパはホスト鍵の確認に応答できないため、初回接続に失敗します。先に Known Hosts でホスト鍵を登録してください。",
  "password.warn.hostNameUnresolved":
    "この alias の HostName を特定できませんでした。パスワードは alias に紐づけて保存されます。",
  "password.show": "表示",
  "password.showNamed": "{label}を表示",
  "password.hideNamed": "{label}を隠す",
  "password.hide": "隠す",
  "sync.heading": "Sync",
  "sync.overviewHeading": "同期",
  "sync.manageSettings": "同期設定を管理",
  "sync.exclusions.heading": "同期するファイル",
  "sync.exclusions.open": "選択・除外ルール",
  "sync.exclusions.loading": "同期対象を読み込んでいます…",
  "sync.exclusions.summary": "対象 {included} 件 · 除外 {ignored} 件",
  "sync.exclusions.hint":
    "チェックを外したファイルは送信せず、受信時にも上書き・削除しません。ローカルにある内容はそのまま残ります。",
  "sync.exclusions.defaults":
    "まだ .sshcignore はありません。OSの管理ファイル、バックアップ、一時ファイル、ロックファイルを除外する既定ルールが有効です。保存すると端末間で共有されます。",
  "sync.exclusions.search": "ファイル名・パスを検索",
  "sync.exclusions.empty": "一致するファイルはありません。",
  "sync.exclusions.sensitiveWarning":
    "接続設定または鍵が除外されています。別の端末ではその接続を再現できない可能性があります。",
  "sync.exclusions.advanced": ".sshcignore を編集",
  "sync.exclusions.rules": "同期除外ルール",
  "sync.exclusions.syntax":
    "Gitignoreと同じ形式で *、**、?、[a-z]、!（再追加）を使えます。.sshcignore自身とsshcの端末固有状態はこの設定に関係なく扱われます。",
  "sync.exclusions.save": "除外設定を保存",
  "sync.exclusions.shared": ".sshcignore は同期され、すべての端末で同じルールを使います。",
  "sync.exclusions.invalid": "除外ルールの書式が正しくありません。",
  "sync.exclusions.loadFailed": "同期対象を読み込めませんでした。",
  "sync.exclusions.saveFailed": "除外設定を保存できませんでした。",
  "sync.receiveRemote": "リモートから受信",
  "sync.autoBlockedReason": "同期を停止しています。理由：{reason}",
  "sync.autoFailedReason": "前回の同期に失敗しました。理由：{reason}",
  "sync.setup.check": "接続を確認",
  "sync.setup.empty": "接続できました。このパスには同期データがありません。",
  "sync.setup.existing": "既存の同期データが見つかりました。",
  "sync.setup.incomplete":
    "履歴はありますが、現在のスナップショットがありません。",
  "sync.setup.useAnotherPath":
    "新しく始める場合は、空の別パスを指定してください。",
  "sync.setup.existingKey":
    "この同期データと同じ暗号化キーを入力してください。保存前に実際のスナップショットを復号して確認します。",
  "sync.setup.emptyKey":
    "強い暗号化キーを自動生成します。自分で決めることもできます。",
  "sync.setup.save": "確認して保存",
  "sync.setup.saved": "接続情報と暗号化キーを保存しました。",
  "sync.setup.changed":
    "確認後に同期先の状態が変わりました。もう一度、接続を確認してください。",
  "sync.role.main": "このマシンで送受信",
  "sync.role.receive": "このマシンでは受信のみ",
  "sync.role.advanced": "同期モードの詳細設定",
  "sync.role.send": "このマシンでは送信のみ",
  "sync.pageDescription":
    "この SSH ワークスペースを暗号化スナップショットとして保存し、オブジェクトストレージ経由でマシン間に同期します。",
  "sync.flowHeading": "同期を始める手順",
  "sync.flowBucket": "バケットを設定",
  "sync.flowKey": "共通の暗号化キーを設定",
  "sync.flowOperate": "状態を確認して送受信",
  "sync.loading": "同期設定を読み込んでいます…",
  "sync.warning":
    "~/.ssh の全ファイル（秘密鍵を含む）が同期対象です。スナップショットは、アップロード前にこのマシン上で下のキーを使って暗号化されるため、ストレージ事業者には平文が送信されません。ただし、バケットの認証情報を持つ第三者は暗号化済みの鍵を取得し、暗号化キーの総当たりをオフラインで続けられます。",
  "sync.statusFailed": "同期設定を読み取れませんでした。",
  "sync.bucketHeading": "バケット",
  "sync.notConfigured": "まだバケットが設定されていません。",
  "sync.endpoint": "エンドポイント",
  "sync.endpointHint":
    "https である必要があります。R2 では https://<account>.r2.cloudflarestorage.com です。",
  "sync.bucket": "バケット名",
  "sync.path": "バケット内のパス",
  "sync.pathHint": "任意です。空ならバケットのルートに置きます。",
  "sync.region": "リージョン",
  "sync.regionHint":
    "任意です。空欄の場合は auto を使用します。R2 では auto、AWS S3 ではバケットのリージョンを指定してください。",
  "sync.accessKeyId": "アクセスキー ID",
  "sync.secretAccessKey": "シークレットアクセスキー",
  "sync.credentialsNote":
    "バケットの認証情報はマスターパスワードで暗号化して保存し、スナップショットには含めません。スナップショットに含めると、1 つのスナップショットを入手した第三者が以後のスナップショットも取得できるためです。",
  "sync.sealed":
    "この設定はマスターパスワードで暗号化されています。表示するには Vault を開いてください。",
  "sync.unlockFailed": "マスターパスワードが違います。",
  "sync.noVault":
    "このマシンにはまだ Vault がありません。「Vault」で作成してから戻ってきてください。",
  "sync.direction": "同期の方向",
  "sync.direction.both": "送信と受信",
  "sync.direction.push": "送信のみ",
  "sync.direction.pull": "受信のみ",
  "sync.direction.both.hint":
    "このマシンの変更を送信し、他のマシンが送信した変更を適用できます。",
  "sync.direction.push.hint":
    "このマシンからの送信のみを行います。他のマシンの変更は適用されませんが、変更内容の確認はできます。",
  "sync.direction.pull.hint":
    "このマシンでは受信のみを行います。ローカルの変更はバケットや他のマシンへ送信されません。",
  "sync.configure": "このバケットを使う",
  "sync.editSettings": "バケット設定を編集",
  "sync.cancelSettings": "編集をキャンセル",
  "sync.configureFailed": "そのバケットを設定できませんでした。",
  "sync.detailsHeading": "詳細・履歴",
  "sync.neverSynced": "このマシンはまだ同期していません。",
  "sync.lastSynced": "最終同期 {at}、{count} ファイル。",
  "sync.key": "暗号化キー",
  "sync.keyHint":
    "スナップショットはマスターパスワードではなく、このキーで暗号化します。同じバケットを共有するすべてのマシンに同じキーを入力してください。マスターパスワードはマシンごとに異なる値を使用できます。このキーを紛失するとスナップショットを復号できません。",
  "sync.keyMissing":
    "キーがまだありません。ここで作り、同じキーを他のマシンにも入力してください。",
  "sync.keySet": "キーは設定済みです。保存後の値は再表示できません。",
  "sync.keyReady": "設定済み",
  "sync.keyNeeded": "キーが必要",
  "sync.keyShownOnce":
    "いまコピーして、他のマシンに入力してください。この画面を離れると二度と表示されません。",
  "sync.keyChooseOwn": "自分で決める",
  "sync.keyOwnValue": "キー",
  "sync.keyCreate": "暗号化キーを作成",
  "sync.keyReplace": "キーを置き換える",
  "sync.keySaved": "保存しました。",
  "sync.keyFailed": "キーを保存できませんでした。",
  "sync.keyTooShort": "自分で決めるキーは 12 文字以上にしてください。",
  "sync.wrongKey":
    "保存済みのキーでは、このバケットのスナップショットを復号できません。すべてのマシンで同じキーを使用しているか確認するか、バケットの状態からリモートスナップショットを明示的に置き換えてください。",
  "sync.wrongMaster": "このマシンのマスターパスワードと違います。",
  "sync.bucketAuthenticationFailed":
    "オブジェクトストレージが要求を認証できませんでした。何も保存していません。アクセスキーとシークレットを確認してください。",
  "sync.bucketAccessDenied":
    "オブジェクトストレージへのアクセスが拒否されました。何も保存していません。資格情報、バケット、リージョン、キーの権限を確認してください。",
  "sync.bucketRateLimited":
    "オブジェクトストレージが要求を制限しています。しばらく待ってから再実行してください。",
  "sync.bucketUnavailable":
    "オブジェクトストレージのサービスが一時的に利用できません。時間を置いて再実行してください。",
  "sync.unreachable":
    "オブジェクトストレージが要求を拒否しました。何も保存していません。バケット名、アクセスキー、シークレット、リージョン、権限を確認してください。",
  "sync.bucketTimeout":
    "オブジェクトストレージが時間内に応答しませんでした。回線とエンドポイントを確認してください。",
  "sync.bucketDNSFailed":
    "エンドポイントのホスト名を解決できませんでした。入力したアドレスと端末のDNS接続を確認してください。",
  "sync.bucketTLSFailed":
    "エンドポイントとの安全な接続を検証できませんでした。HTTPSのアドレスと端末の日時を確認してください。",
  "sync.bucketUnreachable":
    "オブジェクトストレージへ接続できませんでした。端末のネットワークとエンドポイントを確認してください。",
  "sync.snapshotDownloadIncomplete":
    "暗号化スナップショットの受信が途中で終了しました。回線を確認して、もう一度受信してください。",
  "sync.snapshotCostRefused":
    "このスナップショットは復号時の負荷が安全上限を超えるため開けません。",
  "sync.snapshotSchemaUnsupported":
    "このスナップショットは現在のsshcが対応していない形式です。作成した端末のsshcと同じか新しい版へ更新してください。",
  "sync.snapshotRejected":
    "取得したデータは有効なsshcスナップショットではないか、破損しています。何も上書きしていません。",
  "sync.snapshotTooLarge":
    "スナップショットが安全に読み込める上限を超えています。何も上書きしていません。",
  "sync.noSnapshot":
    "指定したバケットとパスに現在のスナップショットがありません。",
  "sync.internalFailed":
    "同期処理で分類できない内部エラーが発生しました。下の診断コードを添えて報告してください。",
  "sync.failed":
    "同期できませんでした。何も上書きしていません。接続を確認してから、もう一度お試しください。",
  "sync.localChanged":
    "確認後にこのマシンの設定が変更されました。上書きせずに停止しました。もう一度「変更を確認」からやり直してください。",
  "sync.workspaceBusy":
    "別の処理がこのマシンの設定を更新しています。完了してから、もう一度お試しください。",
  "sync.endpointPath":
    "エンドポイントはアカウントのアドレスだけです。バケット名やパスは含めません。バケット名は下の欄に入れてください。",
  "sync.auto": "自動同期",
  "sync.autoHint.both":
    "Vault が開いている間、1 分ごとにリモートの更新を確認します。このマシンの設定を変更した場合は、最後の変更から 5 秒後に一度だけ送信します。競合は自動解決せず、ファイルを削除する変更も自動適用しません。どちらの場合も自動同期を停止して通知します。",
  "sync.autoHint.pull":
    "Vault が開いている間、1 分ごとにリモートの更新を確認します。このマシンの変更は送信しません。競合やファイルを削除する変更を見つけた場合は、自動受信を停止して通知します。",
  "sync.autoHint.push":
    "Vault が開いている間、リモートが更新されていないか確認します。このマシンの設定を変更した場合は、最後の変更から 5 秒後に一度だけ送信します。リモートの内容は受信しません。",
  "sync.autoEnable": "このマシンを自動で同期する",
  "sync.autoIdle": "停止中",
  "sync.autoLastRan": "最後に確認したのは {at} です。",
  "sync.autoBlockedConflicts":
    "自動同期を停止しました。同じファイルがこのマシンと他のマシンの両方で変更されています。「変更を確認」で内容を比較してください。",
  "sync.autoBlockedRemovals":
    "自動同期を停止しました。適用すると、このマシンからファイルが削除されます。「変更を確認」で対象を確認してください。",
  "sync.autoBlockedRemoteMoved":
    "別のマシンがリモートスナップショットを変更したため、送信前に自動同期を停止しました。変更を取得するか、確認したリモートスナップショットを明示的に置き換えてください。",
  "sync.autoBlockedRemoteMovedPull":
    "現在のリモートは、このマシンが最後に受信した履歴から分岐しています。意図しない巻き戻しを防ぐため、自動受信を停止しました。",
  "sync.remoteHeadReviewHint":
    "受信専用では、現在のリモートを正として明示的に受信できます。作成日時・作成元・変更されるファイルを確認してから適用します。",
  "sync.remoteHeadReview": "現在のリモートを確認",
  "sync.checkRemoteChanges": "リモートの変更を確認",
  "sync.remoteHeadPreviewHeading": "現在のリモートを受信",
  "sync.remoteHeadPreview":
    "作成 {at}・作成元 {origin} のリモートを確認しています。適用時にも同じ世代であることを再確認し、途中で変わっていた場合は何も書き込みません。",
  "sync.remoteHeadApply": "このリモートを受信",
  "sync.autoBlockedRemoteDeleted":
    "以前同期した現在のスナップショットがバケットから削除されています。意図せず再作成または上書きしないよう、自動同期を停止しました。",
  "sync.autoFailedLast":
    "前回はバケットに接続できませんでした。次回の自動同期で再試行します。",
  "sync.autoFailedWrongKey":
    "このマシンの同期鍵ではリモートスナップショットを開けません。同じスナップショットの自動取得は再試行しません。",
  "sync.autoFailedSchema":
    "リモートスナップショットの形式に対応していません。同じスナップショットの自動取得は再試行しません。",
  "sync.autoFailed": "設定を保存できませんでした。",
  "sync.autoNow": "今すぐ同期",
  "sync.autoNow.both": "今すぐ同期",
  "sync.autoNow.pull": "今すぐ受信",
  "sync.autoNow.push": "今すぐ送信",
  "sync.autoNowFailed": "その確認を実行できませんでした。",
  "sync.transferHeading": "送信・変更確認",
  "sync.transferHint.both":
    "このマシンの内容を送信するか、他のマシンの変更を確認してから適用します。",
  "sync.transferHint.push":
    "このマシンの内容を送信します。リモートの内容はこのマシンへ適用しません。",
  "sync.commitMessage": "コミットメッセージ",
  "sync.commitMessagePlaceholder": "このスナップショットの変更内容",
  "sync.commitMessageHint":
    "ローカル差分から自動生成します。手動送信前に編集できます。",
  "sync.commitMessageChanges":
    "ローカル差分：追加 {added} 件 · 変更 {modified} 件 · 削除 {removed} 件",
  "sync.commitMessageInvalid":
    "コミットメッセージは 240 文字以内の 1 行で入力してください。",
  "sync.bucketStateHeading": "バケットの状態",
  "sync.bucketStateHint":
    "暗号化された内容は取得せず、現在のスナップショットと日付付き履歴を S3 から直接確認します。画面表示中は 30 秒ごと、送受信後にも更新します。",
  "sync.bucketRefresh": "バケットの状態を更新",
  "sync.bucketNotConfigured": "バケットを設定すると状態を確認できます。",
  "sync.bucketLoading": "バケットを確認しています…",
  "sync.bucketStatusFailed": "バケットの状態を取得できませんでした。",
  "sync.bucketLive": "現在のスナップショット",
  "sync.bucketLiveEmpty": "バケットに現在のスナップショットはありません。",
  "sync.bucketLocalCurrent":
    "このマシンは現在のリモートスナップショットを確認済みです。",
  "sync.bucketLocalBehind":
    "リモートスナップショットが、このマシンで最後に確認した内容から更新されています。",
  "sync.bucketHistory": "日付付き履歴 · {count} 件",
  "sync.bucketHistoryShowing": "{count} 件のうち最新 {shown} 件を表示しています。",
  "sync.bucketHistoryExpand": "すべての履歴を表示",
  "sync.bucketHistoryCollapse": "最新 5 件だけ表示",
  "sync.bucketObjectName": "S3 オブジェクト名を表示",
  "sync.bucketHistoryTruncated":
    "最新 10,000 件を表示しています。これより古い履歴もバケットにあります。",
  "sync.bucketHistoryEmpty": "日付付き履歴は見つかりませんでした。",
  "sync.bucketObjectMeta": "{size}・更新 {at}",
  "sync.bucketCheckedAt": "バケット確認時刻 {at}",
  "sync.historyHeading": "暗号化された世代履歴",
  "sync.historyHint":
    "このマシン上で最近の履歴を上限付きで復号し、コミットグラフとして表示します。ファイル内容はAPI応答へ出しません。",
  "sync.historyRefresh": "世代履歴を更新",
  "sync.historyNeedsKey": "共有暗号化キーを設定すると世代履歴を読めます。",
  "sync.historyLoading": "最近の世代を復号しています…",
  "sync.historyFailed": "暗号化されたバックアップ履歴を読み込めませんでした。",
  "sync.historySummary": "{count} 世代 · 取得 {size}",
  "sync.historyTruncated":
    "上限内の新しい履歴だけを復号しています。古い履歴ファイルは上のバケット一覧に残っています。",
  "sync.historySkipped":
    "現在の形式または暗号化キーで開けない履歴ファイルを {count} 件スキップしました。",
  "sync.historyTimeline": "世代タイムライン",
  "sync.historyRelation.head": "HEAD",
  "sync.historyRelation.ancestor": "祖先",
  "sync.historyRelation.branch": "分岐",
  "sync.historyRevisionMeta": "{at} · {count} ファイル · 作成元 {origin}",
  "sync.historyParent": "親 {revision}",
  "sync.historySelect":
    "世代を選ぶと、現在のリモート最新版との差分を確認できます。",
  "sync.historySelected": "選択した世代",
  "sync.historyDiffLoading": "パスの差分を確認しています…",
  "sync.historyDiffEmpty": "この世代をもう一度選ぶとパスの差分を確認できます。",
  "sync.historyDiffFailed": "選択した世代との差分を確認できませんでした。",
  "sync.historyDiff.added": "追加 {count} 件",
  "sync.historyDiff.modified": "変更 {count} 件",
  "sync.historyDiff.removed": "削除 {count} 件",
  "sync.historyRestoreHint":
    "復元前にローカルの変更内容を確認します。リモートの最新版は巻き戻さず、次の送信でこの世代を親にした新しい最新版を作ります。",
  "sync.historyRestorePreview": "この世代の復元を確認",
  "sync.forceHeading": "リモートスナップショットを置き換える",
  "sync.forceHint":
    "読み取れない、または不要なスナップショットがあるバケットへ変更した場合に使います。このワークスペースを暗号化し、上に表示した現在のリモートスナップショットだけを置き換えます。確認後に内容が変わった場合は中止します。既存の日付付き履歴は削除しません。",
  "sync.forceConfirm":
    "現在のリモートスナップショットを、このマシンのワークスペースで置き換えることを理解しました。",
  "sync.forcePush": "リモートスナップショットを置き換える",
  "sync.forcePushShort": "強制送信",
  "sync.forcePull": "強制受信",
  "sync.dialogClose": "閉じる",
  "sync.forcePushed": "確認したリモートスナップショットを置き換えました。",
  "sync.forceFailed":
    "リモートスナップショットを置き換えられませんでした。バケットの状態を更新し、もう一度確認してください。",
  "sync.push": "このワークスペースを送信",
  "sync.pushed": "ワークスペースを送信しました。",
  "sync.pushFailed":
    "スナップショットを送信できませんでした。他のマシンがこのマシンの最終同期後に送信している場合は、先にその変更を取得してください。",
  "sync.noLocalChanges": "送信するローカル変更はありません。",
  "sync.remoteMoved":
    "別のマシンが最新のスナップショットを変更したため、再送信せず停止しました。先に変更を取得するか、確認したリモートスナップショットを明示的に置き換えてください。",
  "sync.previewStale":
    "プレビュー後にリモートのスナップショットが変わりました。もう一度プレビューしてから適用してください。",
  "sync.remoteDeleted":
    "適用前に現在のリモートスナップショットが削除されました。バケットの状態を更新してください。",
  "sync.keyRecoveryRequired":
    "同期キーの置き換えが中断されました。同じ新しい同期キーをもう一度入力して復旧してください。",
  "sync.keyRecoveryTargetChange":
    "中断された同期キーの置き換えを完了してから、バケットまたはパスを変更してください。",
  "sync.keyHistoryLossConfirm":
    "過去の履歴スナップショットは以前のキーで暗号化されたままとなり、読み取れなくなることを理解しました。",
  "sync.preview": "変更を確認",
  "sync.pullFailed": "スナップショットを読み取れませんでした。",
  "sync.alreadyMatches":
    "このワークスペースは既にスナップショットと一致しています。",
  "sync.previewHeading": "取得した場合の変更",
  "sync.conflictExplain":
    "これらのファイルは、このマシンと他のマシンの両方で変更されています。同じ設定ブロックを自動的に統合できないため、何も適用していません。設定ファイル画面で手動で統合するか、採用する内容を持つマシンから送信してください。",
  "sync.conflictPermissions":
    "権限: 前回の同期 {base}・このマシン {local}・リモート {remote}",
  "sync.keepMine": "このマシンの内容を残す",
  "sync.takeTheirs": "他のマシンの内容を使用",
  "sync.wouldWrite": "{count} 個のファイルが書き込まれます:",
  "sync.wouldRemove": "{count} 個のファイルが削除されます:",
  "sync.confirmOverwrite":
    "~/.ssh のファイルを上書きし、上記のファイルをこのマシンから削除します。変更対象は事前に ~/.ssh/sshc/backups/ へバックアップされ、履歴から復元できます。続行しますか？",
  "sync.apply": "スナップショットを適用",
  "sync.applied": "適用しました。この変更は履歴に残り、あとから取り消せます。",
  "sync.applyFailed": "スナップショットを適用できませんでした。",
  "sync.result.pushTitle": "今回の送信",
  "sync.result.previewTitle": "受信内容の確認",
  "sync.result.applyTitle": "適用結果",
  "sync.result.previousTitle": "前回の成功",
  "sync.result.filesSource": "{count} ファイル · {size}",
  "sync.result.encrypted": "暗号化スナップショット {size}",
  "sync.result.uploaded":
    "S3 転送 {size}（{count} オブジェクト、履歴＋現在版）",
  "sync.result.previewDownload": "{downloaded} 取得 · 展開後 {source}",
  "sync.result.applyDownload": "適用時に再取得 {size}",
  "sync.result.changes":
    "書き込み {written} 件 · 削除 {removed} 件 · 競合 {conflicts} 件",
  "sync.result.created": "スナップショット作成 {at}",
  "sync.result.snapshotAt": "{at}のスナップショット",
  "sync.result.appliedSnapshot": "{at}のスナップショットを適用",
  "sync.result.completed": "操作完了 {at}",
  "diag.heading": "Diagnostics",
  "diag.pageDescription":
    "保存済みの接続先だけでなく、一時的に指定したホストも検査できます。検査を開始するまで接続しません。",
  "diag.configUnreadable": "設定を読み取れませんでした。",
  "diag.running": "選択した検査を実行しています…",
  "diag.idle": "開始するまで、どの検査も実行されません。",
  "diag.hostAlias": "Host alias",
  "diag.needsAlias": "検査するには Host alias を入力してください。",
  "diag.explain": "設定を解析",
  "diag.explainFailed": "この alias の設定を解析できませんでした。",
  "diag.checkReachability": "疎通を確認",
  "diag.reachabilityFailed": "疎通確認を実行できませんでした。",
  "diag.testAuthentication": "認証を確認",
  "diag.authenticationFailed": "認証テストを実行できませんでした。",
  "diag.configuration": "設定ファイル",
  "diag.missingSuffix": "（存在しません）",
  "diag.canRunCommand": "この設定ではコマンドを実行できます",
  "diag.directiveAt": "{keyword}（{path}:{line}）",
  "diag.sourcesCaption":
    "各値の参照元。「不採用」と表示された行は、採用された行より後に読み込まれたため効果がありません。",
  "diag.tableScrollHint": "横にスワイプするとすべての列を確認できます",
  "diag.columnKeyword": "キーワード",
  "diag.columnValue": "値",
  "diag.columnWhere": "読み込み元",
  "diag.columnCondition": "条件",
  "diag.columnState": "状態",
  "diag.inEffect": "有効",
  "diag.superseded": "不採用",
  "diag.route": "接続経路",
  "diag.hopComplex":
    "このホップは alias だけで指定されていないため、接続先をここでは解決できません",
  "diag.reachedThrough": "{parent} を経由",
  "diag.notSimple": "この設定は簡易表示では解決できません",
  "diag.notSimpleDetail":
    "sshc では各値の参照元だけを表示します。最終的な値は `ssh -G` で確認してください。",
  "diag.inside": "{condition} の内側",
  "diag.reachability": "疎通",
  "diag.authentication": "認証",
  "diag.authenticationMethod": "{method} で認証されました。",
  "diag.forHost": "{host} の診断",

  "kh.heading": "Known Hosts",
  "kh.pageDescription":
    "信頼済みホスト鍵を確認し、接続先をスキャンして、known_hosts へ追加する前にフィンガープリントを検証します。",
  "kh.metricEntries": "信頼済みエントリ",
  "kh.metricHashed": "ハッシュ化ホスト",
  "kh.metricCandidates": "スキャン候補",
  "kh.scanHeading": "ホストをスキャン",
  "kh.trustedHeading": "信頼済みホスト鍵",
  "kh.columnHost": "ホスト",
  "kh.columnType": "鍵種別",
  "kh.columnFingerprint": "フィンガープリント",
  "kh.columnTrust": "信頼状態",
  "kh.columnActions": "操作",
  "kh.unreadable": "known_hosts を読み取れませんでした。",
  "kh.removeFailed": "エントリを削除できませんでした。何も変更していません。",
  "kh.scanFailed": "このホストをスキャンできませんでした。",
  "kh.addFailed": "鍵を追加できませんでした。何も変更していません。",
  "kh.addFailedCode":
    "鍵を追加できませんでした（{code}）。何も変更していません。",
  "kh.removed": "1 件のエントリを削除しました（履歴 ID：{id}）。",
  "kh.added": "{host} を追加しました（履歴 ID：{id}）。",
  "kh.search": "検索",
  "kh.hashed": "（ハッシュ済み）",
  "kh.delete": "削除",
  "kh.confirmRemove":
    "{line} 行目（{fingerprint}）を削除しますか？ 操作は履歴に記録され、バックアップも保存されます。",
  "kh.confirmDelete": "削除を確定",
  "kh.cancel": "キャンセル",
  "kh.hostToScan": "スキャンするホスト",
  "kh.scan": "スキャン",
  "kh.scanCandidates": "スキャン候補",
  "kh.unverified": "未検証",
  "kh.add": "追加",
  "kh.addHeading": "未検証のホスト鍵を追加",
  "kh.addExplain":
    "{host} への接続時にこの鍵が提示されましたが、通信を傍受した第三者によって差し替えられた可能性があります。別の信頼できる経路で入手したフィンガープリントを入力するか、未検証の鍵を信頼するリスクを確認してください。",
  "kh.expectedFingerprint": "別経路で確認したフィンガープリント",
  "kh.acknowledge":
    "検証できていないリスクを理解したうえで、この鍵を信頼します",
  "kh.addToKnownHosts": "known_hosts に追加",
  "kh.fingerprintMismatch":
    "入力したフィンガープリントはこの鍵と一致しません。入力値：{typed}、スキャン結果：{scanned}。何も追加していません。",

  "rk.heading": "Remote Keys",
  "rk.pageDescription":
    "公開鍵をリモートアカウントへ登録する前に、authorized_keys へ追加する内容を確認します。",
  "rk.waiting": "サーバーの応答を待っています…",
  "rk.idle": "確認するまで、リモートホストへは何も送られません。",
  "rk.added": "リモートの authorized_keys に鍵を追加しました。",
  "rk.alreadyPresent":
    "鍵は既に存在したため、リモートのファイルはそのままです。",
  "rk.valuesFromEngine":
    "sshc が設定を読んだ結果です（ssh は実行していません）",
  "rk.valuesFromSshG": "ssh -G が解決した結果です（OpenSSH 自身によるもの）",
  "rk.pickFromSsh": "~/.ssh から公開鍵を選ぶ",
  "rk.typeInstead": "公開鍵を手動で入力",
  "rk.hostAlias": "Host alias",
  "rk.hostSearch": "接続先",
  "rk.hostSearchPlaceholder": "ホストを検索、または alias を入力",
  "rk.hostTypeHint":
    "上に Host alias を入力してください。保存済みの接続があればここに表示されます。",
  "rk.hostsSelected": "{count} 件を選択中",
  "rk.hostChoices": "接続先の候補",
  "rk.selectMatches": "一致項目を選択",
  "rk.clearSelection": "解除",
  "rk.noHostMatches": "一致する接続先がありません",
  "rk.chooseHost":
    "接続先を 1 件以上選択するか、Host alias を入力してください。",
  "rk.publicKeyFile": "公開鍵ファイル",
  "rk.publicKeyLine": "公開鍵の行",
  "rk.showWhatWouldHappen": "登録内容を確認",
  "rk.register": "鍵を登録",
  "rk.registerMany": "{count} 台へ鍵を登録",
  "rk.plannedHosts": "{count} 台の内容を確認済み",
  "rk.planFailed":
    "登録内容を確認できませんでした。どのホストにも接続していません。",
  "rk.registerFailed":
    "鍵は登録されませんでした。リモートホストはそのままです。",
  "rk.publicKeyUnreadable":
    "その公開鍵を読み取れませんでした。どこにも接続していません。",
  "rk.withCode": "{message}（{code}）",
  "rk.confirmHeading": "リモート登録の確認",
  "rk.confirmManyHeading": "{count} 台への登録内容を確認",
  "rk.planFor": "{alias} への登録内容",
  "rk.alias": "Alias",
  "rk.effectiveUser": "接続ユーザー",
  "rk.noUser": "設定にありません。ssh はローカルのアカウント名を使います",
  "rk.destination": "接続先",
  "rk.valuesCameFrom": "設定値の参照元",
  "rk.keyFile": "公開鍵ファイル",
  "rk.fingerprint": "フィンガープリント",
  "rk.appendTo":
    "{hostname} の {account} にある {remotePath} へ、同じ行が存在しない場合のみ 1 行追加します。",
  "rk.theRemoteAccount": "リモートアカウント",
  "rk.usersAccount": "{user} のアカウント",
  "rk.keyLineLabel": "追加する公開鍵の行",
  "rk.remoteRuns":
    "リモートホストで次のコマンドを実行し、鍵を標準入力へ渡します:",
  "rk.remoteCommandLabel": "リモートコマンド",
  "rk.connectingRuns": "接続時にリモートコマンドが実行されます",
  "rk.acknowledgeRuns": "実行されるリモートコマンドを確認しました",
  "rk.manualHeading":
    "sshc からこのホストへ鍵を登録できません。次の手順を手動で実行してください:",
  "rk.result": "結果",
  "rk.someRegistrationsFailed":
    "更新できなかった接続先があります。接続先ごとの結果を確認してください。",

  "explorer.loading": "設定ファイルを読み込んでいます…",
  "explorer.pageTitle": "SSH Config",
  "explorer.pageDescription":
    "Include の参照関係を確認し、OpenSSH の書式を維持したまま設定ファイルとワークスペース内のファイルを管理します。",
  "explorer.metricFiles": "読み込んだファイル",
  "explorer.metricEditable": "編集可能",
  "explorer.metricDiagnostics": "診断",
  "explorer.hierarchy": "Include 階層",
  "explorer.externalFile":
    "このファイルは ~/.ssh の外にあります。読み取って表示するだけで、書き込みは行いません。",
  "explorer.insideCondition": "{condition} の内側",
  "explorer.fileState": "{missing}{loads}{editable}",
  "explorer.missing": "存在しません · ",
  "explorer.readTimes": "{count} 回読まれます · ",
  "explorer.editable": "編集可能",
  "explorer.readOnly": "読み取り専用",
  "explorer.newFilePath": "新しいファイルのパス",
  "explorer.workspaceActions": "ワークスペースのファイル",
  "explorer.directoryHelp": "ファイルとディレクトリの操作",
  "explorer.createFile": "ファイルを作成",
  "explorer.createDirectory": "ディレクトリを作成",
  "explorer.deleteDirectory": "ディレクトリを削除",
  "explorer.directoryNote":
    "ディレクトリを作成・削除できます。削除できるのは空のディレクトリだけです。先に中のファイルを削除してください。ファイルを直接参照する Include 行も同時に更新されます。宣言済みグループはグループ画面で管理してください。",
  "explorer.fileOperations": "このファイル",
  "explorer.fileOperationsNote":
    "ファイル名を変更すると、このファイルを直接参照する Include 行も同時に書き換えます。ワイルドカードのパターンは変更せず、新しいパスが一致しない場合は警告します。",
  "explorer.renameTo": "新しいパス",
  "explorer.renameFile": "ファイル名を変更",
  "explorer.deleteFile": "ファイルを削除",
  "explorer.confirmDelete": "削除する",
  "explorer.cancelDelete": "やめる",
  "explorer.deleteIsRecoverable":
    "削除すると、このファイルを直接参照する Include 行も削除します。バックアップは保存されるため、履歴から復元できます。",
  "explorer.saveOrDiscardFirst":
    "未保存の編集があります。リネームや削除の前に保存するか、ファイルを開き直してください。",
  "explorer.newFileNote":
    "新しいファイルを OpenSSH に読み込ませるには、~/.ssh/config の Include から参照してください。グループ間の移動は接続画面で行います。任意のファイルやディレクトリの名前変更・削除には、履歴へ記録できるディレクトリ操作が必要なため、現在は対応していません。",
  "explorer.diagnostics": "診断",
  "explorer.noIncludeProblem": "Include の問題は検出されていません。",
  "explorer.opened": "{path} の {line} 行目を開きました。",
  "explorer.selectFile": "全文を編集するファイルを選んでください。",
  "explorer.emptyHeading": "設定ファイルを選択",
  "explorer.unsaved": "未保存の変更があります",
  "explorer.fileText":
    "ファイル本文 — {path}。入力内容をそのまま書き戻します。",
  "explorer.preview": "プレビュー",
  "explorer.saveFile": "ファイルを保存",

  "groups.loading": "グループを読み込んでいます…",
  "groups.pageTitle": "Groups",
  "groups.pageDescription":
    "接続と鍵を階層化して整理し、グループ単位で共通の SSH 設定を適用します。",
  "groups.metricGroups": "グループ",
  "groups.metricConnections": "Connections",
  "groups.metricDraft": "未保存の下書き",
  "groups.empty": "グループはまだありません。下のフォームから作成できます。",
  "groups.nameTaken":
    "同名のグループが既に存在します。別の名前を指定してください。",
  "groups.chooseGroupAndKeyword":
    "グループとディレクティブのキーワードを選んでください。",
  "groups.unbalancedQuote":
    "値の引用符が対応していません。OpenSSH は引用符の中でエスケープを持たないため、これは保存できません。",
  "groups.renameNeedsName": "名前を変更するには、新しい名前が必要です。",
  "groups.renameCollides":
    "{name} は既に存在します。別の名前にするか、どちらか一方を削除してください。",
  "groups.compileNote":
    "グループは {file} の中の通常の Host ブロックとして生成されます。OpenSSH は最初に読んだ値を保持するため、子グループを親より先に書き出します。",
  "groups.members": "メンバー",
  "groups.noMembers": "なし",
  "groups.colour": "色",
  "groups.clearColour": "{name} の色を消す",
  "groups.renameTo": "{name} の新しい名前",
  "groups.renameShort": "新しい名前",
  "groups.rename": "{name} の名前を変更",
  "groups.displayOrder": "表示順",
  "groups.hide": "{name} を接続タブで非表示",
  "groups.hideOnlyContainers":
    "このグループには直下の接続があります。非表示にするとその接続も見えなくなるため、先に子グループへ移してください。",
  "groups.remove": "{name} を削除",
  "groups.newName": "新しいグループ名",
  "groups.invalidName":
    "グループ名は相対ディレクトリパスです。英数字とドット・ハイフン・アンダースコアをスラッシュで区切り、6 階層までにしてください。",
  "groups.directoryNote":
    "グループはディレクトリとして保存されます。接続先は {connections}/<group>/、鍵は {keys}/<group>/ に配置され、~/.ssh/config にはグループごとに Include 行が生成されます。読み込み順は glob の展開順に依存せず、設定ファイルに明示されます。",
  "groups.howItWorks": "グループと SSH ファイルの仕組み",
  "groups.directories": "{connections}/ · {keys}/",
  "groups.nestingNote":
    "スラッシュで階層化できます。work/eu は work の子グループです。",
  "groups.addChild": "{name} の中にグループを追加",
  "groups.listLabel": "グループ一覧（親が先）",
  "groups.orderNote":
    "ここでは親が先に来るツリー順で表示しています。Include 行は逆順（深い子グループが先）で書かれます。OpenSSH は最初に読んだ値を採用するため、子の設定を親より先に読ませる必要があるからです。",
  "groups.removeInto": "{name} を削除",
  "groups.removeIntoShort": "中の接続の移動先",
  "groups.removeIntoNone": "グループなし（connections/ 直下）",
  "groups.removeExplain":
    "{name} を削除すると、Include 行とグループ設定がなくなります。中の接続 {count} 件はどこかへ移す必要があります。",
  "groups.removeExplainEmpty":
    "{name} を削除すると、Include 行とグループ設定がなくなります。中に接続はないので、移動するものはありません。",
  "groups.removeKeepsFiles":
    "設定ファイルは 1 つも削除されません。1 回の変更として履歴に残るため、あとから元に戻せます。",
  "groups.removeConfirm": "{name} を削除する",
  "groups.removeCancel": "キャンセル",
  "groups.unsaved": "未保存",
  "groups.unsavedBarLabel": "未保存のグループ変更",
  "groups.unsavedBarNote":
    "グループ追加と表示設定はまだ保存されていません。名前変更や削除の前に保存するか破棄してください。",
  "groups.discard": "グループ変更を破棄",
  "groups.saveDraftFirst":
    "保留中のグループ変更を保存または破棄してから、名前変更や削除を行ってください。",
  "groups.savedNote":
    "未保存の変更はありません。色、表示順、グループ、設定の追加は「グループを保存」で反映されます。名前の変更と削除は実行時に即座に書き込まれます。",
  "groups.immediateActions": "名前変更と削除は即座にディスクへ書き込みます。",
  "groups.newGroupNote":
    "このグループのディレクトリはまだありません。保存時に作成され、それまでは名前変更・削除できません。",
  "groups.addHeading": "グループを追加する",
  "groups.add": "グループを追加",
  "groups.settingHeadingFor": "{name} に設定を追加",
  "groups.directive": "ディレクティブ",
  "groups.value": "値",
  "groups.addSetting": "設定を追加",
  "groups.previewChanges": "グループ変更をプレビュー",
  "groups.save": "グループを保存",

  "tree.navLabel": "Connections",
  "tree.ungrouped": "未分類",
  "tree.arrangeBy": "接続の並べ方",
  "tree.byGroups": "グループ",
  "tree.byFiles": "ファイル",
  "tree.groupFilter": "グループを選択",
  "tree.groupSection": "{name} グループ（接続先 {count} 件）",
  "tree.filter": "接続を絞り込む",
  "tree.filterPlaceholder": "alias、パターン、グループ、タグ",
  "tree.filterPlaceholderExpanded":
    "名前、接続先、ユーザー、グループ、タグを検索",
  "tree.allConnections": "すべて",
  "tree.resultsLabel": "接続先",
  "tree.resultCount": "{visible} 件 / 全 {total} 件",
  "tree.sortLabel": "接続の並び順",
  "tree.sortConfigured": "設定順",
  "tree.sortName": "名前順",
  "tree.sortGroup": "グループ順",
  "tree.noMatch": "この条件に一致する接続はありません。",
  "tree.groupEmpty": "このグループに接続はありません。",
  "tree.collapse": "{name} を折りたたむ",
  "tree.expand": "{name} を展開する",
  "tree.patternRuleExternal":
    "{path} のパターン規則です。このエディタはこのファイルを読み取るだけです。",
  "tree.patternRuleOpen":
    "パターン規則 — 設定ファイル画面で開く（{path}:{line}）",
  "tree.duplicateAlias": "alias 重複",
  "tree.patternRule": "パターン規則",
  "tree.dragGroupHint":
    "グループをドラッグすると、階層と表示順を変更できます。",

  "browser.modeLabel": "接続の表示方法",
  "browser.servers": "サーバー",
  "browser.groups": "グループ",
  "browser.groupPath": "グループの場所",
  "browser.ungrouped": "未分類",
  "browser.groupCountOne": "サーバー 1 台",
  "browser.groupCountMany": "サーバー {count} 台",
  "browser.noMatches": "現在の条件に一致するサーバーはありません。",
  "browser.emptyGroup": "このグループ直下にサーバーはありません。",
  "browser.emptyGroups": "宣言済みグループはまだありません。",
  "browser.groupMissing": "グループが見つかりません。",
  "browser.backToGroupRoot": "グループ一覧へ戻る",
  "browser.invalidUrl": "この接続 URL は認識できません。",
  "browser.backToServers": "サーバー一覧へ戻る",
  "browser.duplicateAlias": "alias 重複",

  "conn.loading": "接続を読み込んでいます…",
  "conn.heading": "Connections",
  "conn.count": "接続先 {count} 件",
  "conn.new": "新しい接続",
  "conn.allConnections": "接続一覧",
  "conn.createAnother": "接続を追加",
  "conn.cancelCreate": "キャンセル",
  "conn.create": "接続を作成",
  "conn.createTitle": "接続を作成",
  "conn.createDescription": "SSH 接続先の設定を保存し、認証方法を選択します。",
  "conn.createConnectionSection": "接続先",
  "conn.createName": "接続名",
  "conn.createNameRequired": "接続名（必須）",
  "conn.createGroup": "保存先グループ",
  "conn.createManageGroups": "グループを管理",
  "conn.createNoGroup": "グループなし",
  "conn.createHostName": "ホスト名または IP アドレス",
  "conn.createHostNameRequired": "ホスト名または IP アドレス（必須）",
  "conn.createUser": "ユーザー（任意）",
  "conn.createPort": "ポート（任意）",
  "conn.createPortHint": "空欄の場合は 22 を使用します。",
  "conn.createAuthenticationSection": "認証",
  "conn.createAuthenticationMethod": "認証方法",
  "conn.createDedicatedPassword": "この接続専用の暗号化パスワード",
  "conn.createSavedPassword": "保存済みパスワード",
  "conn.createNewSharedPassword": "新しい保存済みパスワード",
  "conn.createIdentityFile": "SSH 秘密鍵",
  "conn.createConnectionPassword": "接続パスワード",
  "conn.createDedicatedHint":
    "Vault 内で暗号化され、再利用可能なパスワード一覧には表示されません。",
  "conn.createChooseSavedPassword": "保存済みパスワード",
  "conn.createSavedHint":
    "選択した再利用可能なパスワードをこの接続と共有します。",
  "conn.createNoSavedPasswords": "保存済みパスワードはありません",
  "conn.createSavedPasswordName": "保存するパスワードの名前",
  "conn.createNewPassword": "新しいパスワード",
  "conn.createPrivateKey": "SSH 秘密鍵",
  "conn.createNoPrivateKeys": "利用できる秘密鍵がありません",
  "conn.createNoPrivateKeysHint":
    "パスワードを使わない場合は、先に秘密鍵を作成してください。",
  "conn.createCreatePrivateKey": "秘密鍵を作成",
  "conn.createLoadingOptions": "認証の選択肢を読み込んでいます…",
  "conn.createOptionsFailed": "認証の選択肢を読み込めませんでした。",
  "conn.createMasterPassword": "マスターパスワード",
  "conn.createConfirmMaster": "マスターパスワードの確認",
  "conn.createInitialiseVault": "暗号化 Vault を作成",
  "conn.createUnlockVault": "Vault を開く",
  "conn.createVaultMissing":
    "この接続を保存する前に暗号化 Vault を作成してください。",
  "conn.createVaultLocked":
    "この接続を保存する前に暗号化 Vault を開いてください。",
  "conn.createVaultFailed": "暗号化 Vault を作成できませんでした。",
  "conn.createUnlockFailed": "暗号化 Vault を開けませんでした。",
  "conn.createNeedVault": "続行するには暗号化 Vault を開いてください。",
  "conn.createNeedConnectionPassword":
    "続行するには接続パスワードを入力してください。",
  "conn.createNeedSavedPassword":
    "続行するには保存済みパスワードを選んでください。",
  "conn.createNeedSavedPasswordName": "続行するには保存名を入力してください。",
  "conn.createNeedNewPassword":
    "続行するには新しいパスワードを入力してください。",
  "conn.createNeedPrivateKey": "続行するには秘密鍵を選ぶか作成してください。",
  "conn.createDraftWaiting": "{alias} の接続設定を編集中です。",
  "conn.createUntitledDraft": "名前未設定の接続",
  "conn.createReturnToDraft": "接続設定に戻る",
  "conn.createAliasRequired": "接続名を入力してください。",
  "conn.createAliasInvalid":
    "英数字、ピリオド、ハイフン、アンダースコアを使い、先頭は英数字にしてください。",
  "conn.createHostRequired": "ホスト名または IP アドレスを入力してください。",
  "conn.createHostInvalid":
    "DNS 名、IPv4 アドレス、または角括弧なしの IPv6 アドレスを入力してください。",
  "conn.createUserInvalid": "ユーザーには空白や制御文字を含められません。",
  "conn.createPortInvalid": "ポートは 1〜65535 の整数にしてください。",
  "conn.creating": "作成しています…",
  "conn.createFailed": "接続を作成できませんでした。",
  "conn.createAliasTaken": "別の接続がすでにその名前を使っています。",
  "conn.duplicateAliasTaken": "{alias} は既に存在するため複製できません。",
  "conn.createGroupMissing":
    "そのグループは宣言されていません。再読み込みして別のグループを選んでください。",
  "conn.createKeyInvalid":
    "その秘密鍵は利用できなくなりました。再読み込みして別の鍵を選んでください。",
  "conn.createCredentialMissing":
    "その保存済みパスワードは利用できなくなりました。再読み込みして別のものを選んでください。",
  "conn.createDestinationExists":
    "このグループには同名の接続ファイルがすでにあります。",
  "conn.basicConnection": "接続先",
  "conn.basicAuthentication": "認証",
  "conn.basicHostName": "ホスト名または IP アドレス",
  "conn.basicServerKeyInvalid":
    "その SSH 秘密鍵は選択できなくなりました。再読み込みして選び直してください。",
  "conn.basicCredentialExists":
    "同じ名前の保存済みパスワードがあります。「保存済みパスワード」から選ぶか、別の名前を使ってください。",
  "conn.basicCredentialMissing":
    "その保存済みパスワードは存在しなくなりました。再読み込みして選び直してください。",
  "conn.basicPasswordMissing":
    "この接続には削除できる保存済みパスワードがありません。",
  "conn.basicUser": "ユーザー",
  "conn.basicPort": "ポート",
  "conn.basicPrivateKey": "SSH 秘密鍵",
  "conn.basicStoredPassword": "保存済みパスワード",
  "conn.basicThisConnection": "この接続に設定されています。",
  "conn.basicInheritedFrom": "{path}:{line} から継承しています。",
  "conn.basicSSHDefault":
    "SSH の既定値です。他の項目を保存しても、この値は書き込みません。",
  "conn.basicReadOnlyAdvanced":
    "基本タブでは読み取り専用です。元のディレクティブは詳細タブに残っています。",
  "conn.basicComplex":
    "この接続には {keyword} が複数あります。詳細タブで整理してください。",
  "conn.basicUseInheritedHost": "ホスト名を継承値・既定値に戻す",
  "conn.basicUseInheritedUser": "ユーザーを継承値・既定値に戻す",
  "conn.basicUseInheritedPort": "ポートを継承値・既定値に戻す",
  "conn.basicKeepDirect": "この接続の値を維持",
  "conn.basicAgentOrInherited": "ssh-agent または継承した鍵",
  "conn.basicManageKeyPassphrase": "鍵パスフレーズを保存・変更",
  "conn.basicKeyPassphraseHeading": "保存済みの鍵パスフレーズ",
  "conn.basicKeyPassphraseUnencrypted":
    "この秘密鍵は暗号化されていないため、保存するパスフレーズは不要です。",
  "conn.basicKeyPassphraseNone": "この鍵にはパスフレーズが保存されていません。",
  "conn.basicKeyPassphraseDedicated":
    "この鍵だけのパスフレーズが保存されています。",
  "conn.basicKeyPassphraseShared":
    "この鍵は共有の保存済みパスフレーズ「{name}」を使っています。",
  "conn.basicKeyPassphraseSharedOthers":
    "ほかに {count} 個の鍵でも使われています。",
  "conn.basicKeyPassphraseDetach":
    "ここで保存すると、この鍵専用の値へ切り替わります。共有資格情報と、ほかの鍵への割り当ては変更しません。",
  "conn.basicNewKeyPassphrase": "新しい保存用鍵パスフレーズ",
  "conn.basicConfirmKeyPassphrase": "保存用鍵パスフレーズの確認",
  "conn.basicKeyPassphraseMismatch": "鍵パスフレーズが一致しません。",
  "conn.basicKeyPassphraseStoredNote":
    "これは解錠用の値を保存します。秘密鍵ファイル自体を暗号化しているパスフレーズは変更しません。",
  "conn.basicKeyPassphraseWrong":
    "入力したパスフレーズでは、選択した秘密鍵を解錠できません。",
  "conn.basicKeyPassphraseChanged":
    "選択した秘密鍵が変更されました。再読み込みしてから保存してください。",
  "conn.basicGeneratedKeyStaged":
    "{path} をこの接続の下書きに追加しました。「基本設定を保存」で反映します。",
  "conn.basicCustomKey":
    "この接続は独自の IdentityFile パス {path} を使っています。詳細タブで編集してください。",
  "conn.basicComplexKey":
    "この接続には IdentityFile が複数あります。詳細タブで整理してください。",
  "conn.basicAssignedDedicated":
    "この接続専用のパスワードが割り当てられています。値は表示しません。",
  "conn.basicAssignedNamed": "割り当て済み：{name}",
  "conn.basicNoPassword": "保存済みパスワードは割り当てられていません。",
  "conn.basicPasswordCleanup":
    "保存済みパスワードが割り当てられていますが、現在の SSH 設定では使用されません。基本設定を保存すると、この接続への割り当てだけを解除します。保存済みパスワードと、ほかの接続への割り当ては変更しません。",
  "conn.basicPasswordAction": "保存済みパスワードの操作",
  "conn.basicPasswordUnchanged": "パスワードは変更しない",
  "conn.basicReplaceDedicated": "この接続専用パスワードで置き換える",
  "conn.basicRemovePassword": "保存済みパスワードを外す",
  "conn.basicConfirmRemove": "保存済みパスワードを外すことを確認",
  "conn.basicEmptyPasswordUnchanged":
    "空欄のままならパスワードを変更しません。",
  "conn.basicVaultMissing":
    "基本設定を保存する前に暗号化 Vault を作成してください。",
  "conn.basicVaultLocked":
    "基本設定を保存する前に暗号化 Vault を開いてください。",
  "conn.basicNeedVault":
    "この下書きを保存するには暗号化 Vault を開いてください。",
  "conn.basicPasswordBlocked":
    "現在の SSH 設定では、保存済みパスワードを追加・置換できません。",
  "conn.basicNothingChanged": "基本設定はまだ変更されていません。",
  "conn.basicOptionsFailed": "鍵とパスワードの選択肢を読み込めませんでした。",
  "conn.basicCredentialOptionsFailed":
    "保存済みパスワードの選択肢を読み込めませんでした。",
  "conn.basicSaveFailed":
    "基本設定を保存できませんでした。変更は行われていません。エラーを確認するか、再読み込みしてもう一度試してください。",
  "conn.basicRefreshFailed":
    "設定は保存されましたが、更新後のパスワード状態を読み込めませんでした。この接続を再読み込みしてください。",
  "conn.basicConnectionRefreshFailed":
    "設定は保存されましたが、更新後の接続を読み込めませんでした。この接続を再読み込みしてください。",
  "conn.basicSave": "基本設定を保存",
  "conn.basicSaving": "保存しています…",
  "conn.discardChanges": "変更を破棄",
  "conn.blockMoved":
    "このブロックはディスク上で移動しました。接続を読み込み直してもう一度試してください。",
  "conn.emptyHeading": "接続を選択",
  "conn.emptyHint":
    "一覧からホストを選んで SSH 設定を編集するか、新しい接続を作成してください。",
  "conn.assignKeyHeading": "この鍵を使う接続を選択",
  "conn.assignKeyHint":
    "接続を選ぶと {path} が基本設定の下書きに入ります。保存するまで変更はありません。",
  "conn.missingHeading": "この接続は現在利用できません",
  "conn.missingHint":
    "このリンクの作成後に、名前変更・移動・削除された可能性があります。",
  "conn.backToList": "接続一覧へ戻る",
  "conn.summarySaved": "保存済みの接続",
  "conn.summarySavedState": "保存済み",
  "conn.summaryUnsaved": "未保存の変更あり",
  "conn.summaryGroup": "グループ",
  "conn.summaryNoGroup": "グループなし",
  "conn.summaryPrivateKey": "SSH 秘密鍵",
  "conn.summaryKeyNone": "ssh-agent または継承した鍵",
  "conn.summaryKeyComplex": "IdentityFile が複数あります",
  "conn.summaryKeyUnavailable": "{path} — 鍵の詳細を確認できません",
  "conn.summaryKeyPassphrase": "鍵パスフレーズ",
  "conn.summaryKeyPassphraseNone": "パスフレーズは保存されていません",
  "conn.summaryKeyPassphraseDedicated": "この鍵専用として保存済み",
  "conn.summaryKeyPassphraseNamed": "保存済みパスフレーズ：{name}",
  "conn.summaryKeyPassphraseNotNeeded": "暗号化されていない鍵のため不要",
  "conn.summaryAccountPassword": "アカウントのパスワード",
  "conn.summaryPasswordNone": "保存済みパスワードなし",
  "conn.summaryPasswordDedicated": "この接続専用のパスワードを保存済み",
  "conn.summaryPasswordNamed": "保存済みパスワード：{name}",
  "conn.summaryPasswordCleanup":
    "保存済みパスワードが割り当てられていますが、現在の SSH 設定では使用されません。基本設定を保存すると、この接続への割り当てを解除します。",
  "conn.summaryLocked": "施錠中のため確認できません",
  "conn.summaryUnavailable": "この状態を読み込めませんでした",
  "conn.summaryDraftBlocksActions":
    "下書きを保存または破棄してから、保存済みの接続を使用してください。",
  "conn.summaryRefreshing":
    "保存済みの接続を再読み込みしています。完了後に操作できます。",
  "conn.editorLabel": "接続エディタ",
  "conn.areaBasic": "Basic",
  "conn.areaAnalysis": "Analysis",
  "conn.areaAdvanced": "Advanced",
  "conn.areaSshc": "sshc",
  "conn.checksLabel": "接続確認",
  "conn.checkReachability": "疎通を確認",
  "conn.checkAuthentication": "保存済み設定で認証を確認",
  "conn.checking": "確認しています…",
  "conn.checksExecutableHeading":
    "認証確認で SSH ディレクティブが実行される可能性があります",
  "conn.checksExecutableHint":
    "認証確認を続ける前に、保存済みディレクティブの内容を確認してください。",
  "conn.checksDirectiveAt": "{path}:{line} の {keyword}",
  "conn.checksAcknowledge": "確認して認証を実行",
  "conn.analysisLabel": "Config Analysis",
  "conn.analysisExplained": "保存済み設定から解決した値",
  "conn.analysisExplainedHint":
    "この接続で使用する値です。コマンドを実行せずに解析し、解決できない設定は理由を表示します。",
  "conn.analysisAuthoritative": "各値の参照元",
  "conn.analysisAuthoritativeHint":
    "この接続に関係する設定行を、ファイルと行番号、実際に採用されるかどうかとともに表示します。",
  "conn.analysisRun": "出所を表示",
  "conn.analysisRunning": "読み取っています…",
  "conn.analysisExecutableHeading":
    "ssh -G が Match ディレクティブを実行する可能性があります",
  "conn.analysisSources": "OpenSSH が参照した設定行",
  "conn.advancedLabel": "詳細設定",
  "conn.advancedViews": "詳細設定の表示",
  "conn.advancedViewLabel": "表示",
  "conn.advancedDirectives": "Directives",
  "conn.portForwarding": "ポート転送",
  "conn.forwardLoopbackOnly":
    "待ち受けはこの端末内（127.0.0.1）に限定しますが、同じ端末の別OSユーザーから利用される可能性があります。Remote Forward には対応しません。",
  "conn.forwardNoneSaved": "この接続にはポート転送が保存されていません。",
  "conn.forwardLocal": "Local トンネル",
  "conn.forwardDynamic": "SOCKS プロキシ",
  "conn.forwardType": "種類",
  "conn.forwardListenPort": "ローカルポート",
  "conn.forwardDestination": "転送先",
  "conn.forwardDestinationHint":
    "SSH接続先から見たホスト名またはIPアドレスとポートです。",
  "conn.forwardDynamicHint":
    "接続先は、このSOCKSプロキシを使うアプリが通信ごとに指定します。",
  "conn.forwardAdd": "転送を追加",
  "conn.forwardPendingSave":
    "変更を保存すると、この転送が設定へ書き込まれます。",
  "conn.forwardInvalidPort": "1〜65535のローカルポートを入力してください。",
  "conn.forwardInvalidDestination":
    "転送先を host:port の形式で入力してください。",
  "conn.advancedNoFields": "現在の表示条件に一致する設定はありません。",
  "conn.advancedRawBlocksFields":
    "Raw に未保存の変更があります。保存または破棄してからディレクティブを編集してください。",
  "conn.advancedFieldsBlockRaw":
    "ディレクティブに未保存の変更があります。保存または破棄してから Raw を編集してください。",
  "conn.connect": "接続",
  "conn.opening": "接続中…",
  "conn.duplicate": "接続を複製",
  "conn.manage": "その他の接続操作",
  "conn.manageLabel": "接続の管理",
  "conn.manageIndependent":
    "ここでの操作は、基本設定・詳細設定の変更とは別に保存されます。",
  "conn.manageDraftBlocked":
    "接続の識別情報や保存場所を変更する前に、編集中の下書きを保存または破棄してください。",
  "conn.discardPrompt":
    "未保存の接続設定を破棄して、この接続から移動しますか？",
  "conn.keepEditing": "編集を続ける",
  "conn.reloadConnection": "保存済み接続を再読み込み",
  "conn.moveToFile": "保存ファイル",
  "conn.moveToFilePlaceholder": "保存ファイルを選択…",
  "conn.move": "保存ファイルを変更",
  "conn.storageFileNote":
    "所属グループは sshc 上の整理場所です。保存ファイルを変更すると、実際に書き込む SSH 設定ファイルが変わります。通常は変更する必要はありません。",
  "conn.confirmDelete": "削除する",
  "conn.deleteHeading": "{alias} を削除しますか？",
  "conn.deleteBody":
    "この Host ブロックを設定から削除します。削除後も履歴から復元できます。",
  "conn.deleteCancel": "やめる",
  "conn.delete": "接続を削除",

  "host.tabJump": "Jump Host",
  "host.tabRaw": "Raw",
  "host.unbalancedQuote":
    "値の引用符が対応していません。OpenSSH は引用符の中でエスケープを持たないため、これは保存できません。",
  "host.needsKeyword": "ディレクティブにはキーワードが必要です。",
  "host.dangerousField":
    "{keyword} は、OpenSSH がこのホストを評価するときにコマンドを実行する可能性があります。記述どおり保存され、ここで実行されることはありません。",
  "host.keep": "戻す",
  "host.remove": "削除",
  "host.newDirective": "新しいディレクティブ",
  "host.newValue": "新しい値",
  "host.addDirective": "ディレクティブを追加",
  "host.saveChanges": "変更を保存",
  "host.blockText":
    "ブロック本文。コメント、空行、未知のディレクティブは入力したとおりに書き戻されます。",
  "host.saveBlock": "ブロックを保存",
  "host.noDestination":
    "この Host ブロックはパターンに一致する接続へ設定を適用するもので、単独では接続先を特定できません。診断するには、具体的な alias を持つ接続を開いてください。",
  "host.primaryGroup": "所属グループ",
  "host.groupNone": "なし",
  "host.groupNoneMeans":
    "「なし」を選ぶと接続は ~/.ssh/config の末尾へ戻ります。",
  "host.moveToGroup": "このグループへ移動",
  "host.comment": "コメント",
  "host.commentNote":
    "Host 行の上に設定ファイルへ書き込まれます。sshc を使わずにファイルを読む人にも残ります。",
  "host.commentFromNote":
    "これは sshc だけが保存していたメモです。保存すると設定ファイルへ書き込まれ、メモは廃止されます。",
  "host.saveComment": "コメントを保存",
  "host.colour": "色",
  "host.clearColour": "色を消す",
  "host.displayOrder":
    "表示順 — 小さいほど先に並びます。0 の場合はファイル内の順序を使用します。",
  "host.tags": "タグ（カンマ区切り）",
  "host.renameAlias": "alias の変更",
  "host.rename": "変更",

  "keys.heading": "SSH Keys",
  "keys.pageDescription":
    "秘密鍵、公開鍵、証明書の一覧と参照元を確認し、作成・変更・削除を管理します。",
  "keys.search": "鍵を検索",
  "keys.searchPlaceholder": "ファイル、ホスト、フィンガープリント",
  "keys.metricFiles": "確認済み SSH ファイル",
  "keys.metricPrivate": "秘密鍵",
  "keys.metricAttention": "確認が必要",
  "keys.noMatches": "この検索に一致する鍵はありません。",
  "keys.reading": "ssh ディレクトリを読み込んでいます…",
  "keys.unreadable":
    "ssh ディレクトリを読み取れませんでした。sshc を再起動して、もう一度試してください。",
  "keys.createFailed":
    "鍵を作成できませんでした。名前、アルゴリズム、パスフレーズを確認してください。",
  "keys.passphraseFailed":
    "パスフレーズを変更できませんでした。現在のパスフレーズを確認して、もう一度試してください。",
  "keys.agentFailed":
    "鍵を ssh-agent に追加できませんでした。パスフレーズと ssh-agent の接続状態を確認してください。",
  "keys.publicKeyFailed": "公開鍵を読み取れませんでした。",
  "keys.trashFailed": "鍵をゴミ箱へ移動できませんでした。",
  "keys.restoreFailed": "エントリを復元できませんでした。",
  "keys.restoreRefused": "次の理由により復元できません：{blockers}",
  "keys.purgeFailed": "エントリを完全に削除できませんでした。",
  "keys.tableCaption": "内容とファイル権限を確認した SSH ファイル",
  "keys.inventoryEmpty":
    "~/.ssh にはまだファイルがありません。この画面から鍵を作成できます。",
  "keys.colFile": "ファイル",
  "keys.colKind": "種別",
  "keys.kind.privateKey": "秘密鍵",
  "keys.kind.publicKey": "公開鍵",
  "keys.kind.certificate": "証明書",
  "keys.kind.other": "その他のファイル",
  "keys.kind.unreadable": "読み取り不可",
  "keys.colAlgorithm": "アルゴリズム",
  "keys.colFingerprint": "フィンガープリント",
  "keys.colPermissions": "権限",
  "keys.colUsedBy": "参照元",
  "keys.colState": "状態",
  "keys.stateInAgent": "ssh-agent に登録済み",
  "keys.stateUsedBy": "{count} 件の接続で使用",
  "keys.usedByNothing": "この鍵を参照する接続はありません",
  "keys.inspectorLabel": "鍵の詳細",
  "keys.colActions": "操作",
  "keys.showDetails": "詳細を開く",
  "keys.hideDetails": "詳細を閉じる",
  "keys.actionsHeading": "{path} の操作",
  "keys.keyActions": "鍵の操作",
  "keys.permissionRisk": "ファイル権限が緩すぎます",
  "keys.showPublicKey": "公開鍵を表示",
  "keys.relatedPublicFiles": "公開鍵ファイル（{count}）",
  "keys.showPrivateKey": "秘密鍵を表示",
  "keys.changePassphrase": "パスフレーズを変更",
  "keys.moreActions": "その他の操作",
  "keys.manageStoredPassphrase": "保存済みパスフレーズを管理",
  "keys.storedPassphraseHeading": "保存済みパスフレーズ：{path}",
  "keys.storedPassphraseNote":
    "暗号化された鍵のパスフレーズを sshc Vault へ保存するか、既存の名前付きパスフレーズを割り当てます。値はマスターパスワードで暗号化され、保存後はこの画面に表示されません。",
  "keys.newStoredPassphraseName": "パスフレーズ名",
  "keys.newStoredPassphraseValue": "パスフレーズの値",
  "keys.storeAndUsePassphrase": "保存してこの鍵に割り当てる",
  "keys.storedPassphraseExists":
    "その名前はすでに存在します。上の一覧から選ぶか、新しい名前を指定してください。",
  "keys.unassignPassphrase": "割り当てを解除",
  "keys.storePassphraseFailed":
    "パスフレーズを保存し、この鍵へ割り当てることができませんでした。",
  "keys.unassignPassphraseFailed":
    "この鍵への保存済みパスフレーズの割り当てを解除できませんでした。",
  "keys.addToAgent": "ssh-agent に追加",
  "keys.removeFromAgent": "ssh-agent から削除",
  "keys.agentRemoveFailed":
    "ssh-agent から鍵を削除できませんでした。鍵がすでに削除されているか、削除に必要な公開鍵が存在しない可能性があります。",
  "keys.trashConfirmHeading": "{path} をゴミ箱へ移動",
  "keys.trashExplain": "同じ鍵に属するため、次のファイルをまとめて移動します:",
  "keys.trashReferences":
    "{count} 件の Host ブロックがこの鍵を参照しています。移動後、その IdentityFile 行は存在しないファイルを参照します。ssh はエラーを表示した後、他の認証方法を試します。",
  "keys.trashNoReferences": "この鍵を参照する Host ブロックはありません。",
  "keys.trashIsRecoverable":
    "削除はされません。下のゴミ箱へ移動するだけで、復元できます。",
  "keys.trashConfirm": "ゴミ箱へ移動する",
  "keys.trashCancel": "やめる",
  "keys.moveToTrash": "ゴミ箱へ移動",
  "keys.publicKeyHeading": "公開鍵：{path}",
  "keys.publicKeyLabel": "公開鍵",
  "keys.close": "閉じる",
  "keys.unreadableHeading": "内容を確認できなかったファイル",
  "keys.unreadableNote":
    "これらは ~/.ssh の中にあり、上の表には含まれていません。これらに対して何も変更していません。",
  "keys.unresolvedHeading": "存在しない鍵を指している設定エントリ",
  "keys.agentHeading": "ssh-agent",
  "keys.agentEmpty": "ssh-agent に接続できましたが、鍵は登録されていません。",
  "keys.agentIdentitiesCaption": "ssh-agent に登録されている鍵",
  "keys.colComment": "コメント",
  "keys.agentUnavailable":
    "このプロセスから ssh-agent に接続できないため、鍵を登録できません。ssh-add コマンドと、接続先を示す SSH_AUTH_SOCK の両方が必要です。",
  "keys.agentDelegationsNote":
    "次の設定エントリは、鍵ファイルではなく ssh-agent の鍵を使用します:",
  "keys.registerHeading": "ssh-agent に追加：{path}",
  "keys.registerNote":
    "パスフレーズは ssh-add の標準入力へ渡され、コマンドラインや子プロセスの環境変数には含まれません。sshc はパスフレーズを保存せず、この操作の完了後は保持しません。",
  "keys.keyPassphrase": "鍵のパスフレーズ",
  "keys.lifetime": "保持期間",
  "keys.lifetimeForever": "ssh-agent が終了するまで",
  "keys.lifetimeHour": "1 時間",
  "keys.lifetimeFourHours": "4 時間",
  "keys.lifetimeTwelveHours": "12 時間",
  "keys.useStoredPassphrase": "保存済みのパスフレーズを使う",
  "keys.choosePassphraseName": "— 名前を選ぶ —",
  "keys.useThisPassphrase": "このパスフレーズを使う",
  "keys.usesStoredPassphrase":
    "この鍵は保存済みパスフレーズ「{name}」を使用します。",
  "keys.usesDedicatedPassphrase":
    "この鍵だけのパスフレーズが保存されています。",
  "keys.typedWins":
    "空欄の場合は保存済みのパスフレーズを使用します。入力した値がある場合は、その値を使用します。",
  "keys.assignPassphraseFailed":
    "この鍵に指定したパスフレーズを割り当てられませんでした。",
  "keys.registerSubmit": "鍵を ssh-agent に追加",
  "keys.cancel": "キャンセル",
  "keys.passphraseHeading": "パスフレーズを変更：{path}",
  "keys.passphraseNote":
    "ここで入力したパスフレーズは、この変更にのみ使用され、保存されません。sshc Vault へ保存する場合は、鍵一覧の「パスフレーズを保存」を使用してください。",
  "keys.currentPassphrase": "現在のパスフレーズ",
  "keys.newPassphrase": "新しいパスフレーズ",
  "keys.removePassphrase":
    "パスフレーズを削除する（秘密鍵は暗号化されなくなります）",
  "keys.savePassphrase": "新しいパスフレーズを保存",
  "keys.createHeading": "鍵を作成",
  "keys.generatedHeading": "鍵を作成しました",
  "keys.generatedNext": "{path} を作成しました。次の操作を選んでください。",
  "keys.assignGenerated": "接続に割り当てる",
  "keys.installGenerated": "サーバーに公開鍵を登録",
  "keys.algorithm": "アルゴリズム",
  "keys.fileName": "ファイル名",
  "keys.comment": "コメント",
  "keys.passphrase": "パスフレーズ",
  "keys.createUnencrypted":
    "パスフレーズなしで作成する（秘密鍵ファイルを読み取れるユーザーは、この鍵を使用できます）",
  "keys.createSubmit": "鍵を作成",
  "keys.showTerminalCommand": "Terminal コマンドを表示",
  "keys.hardwareNote":
    "ハードウェアに保存する鍵の作成にはセキュリティキーの操作が必要なため、sshc では対応していません。次のコマンドをターミナルで実行してください：",
  "keys.trashHeading": "ゴミ箱",
  "keys.trashSummary": "ゴミ箱（{count}）",
  "keys.trashNote":
    "ゴミ箱へ移動した鍵は、手動で完全削除するまで残ります。自動削除されません。",
  "keys.trashCaption": "ゴミ箱内の鍵",
  "keys.trashEmpty": "削除されたものはありません。",
  "keys.colFiles": "ファイル",
  "keys.colAge": "経過",
  "keys.colStatus": "状態",
  "keys.ageStale": "{days} 日 · 保持期間 {retention} 日を超過",
  "keys.age": "{days} 日",
  "keys.restorable": "復元可能",
  "keys.restore": "復元",
  "keys.purgeWarning":
    "これは取り消せません。完全に削除した鍵のバックアップはありません。",
  "keys.confirmPurge": "完全削除を確定",
  "keys.purge": "完全に削除",
  "keys.noteFingerprintUnavailable": "フィンガープリントを取得できません",
  "keys.noteSymbolicLink": "シンボリックリンク（追跡しません）",
  "keys.noteEmptyFile": "空のファイル",
  "keys.noteNotRegularFile": "通常ファイルではありません",
  "keys.noteCommentNotPreserved": "コメントは保持されていません",
  "keys.certKeyId": "Key ID：{keyId}",
  "keys.certFor": "対象プリンシパル：{principals}",
  "keys.certAnyPrincipal": "すべてのプリンシパルが対象",
  "keys.certNeverExpires": "無期限",
  "keys.certExpired": "{when} に失効",
  "keys.certValidUntil": "{when} まで有効",
  "keys.certSigns": "{keyType} {fingerprint} に署名",
  "keys.reference": "{directive} {value} — {path}:{line}",
  "keys.referenceWithReason": "{directive} {value} — {path}:{line}（{reason}）",
  "keys.unreadableEntry": "{path} — {reason}",

  "keys.colChoose": "選択",
  "keys.chooseKey": "{path} を選ぶ",
  "keys.listFilter": "一覧に表示するファイル",
  "keys.listFilterKeys": "鍵だけ",
  "keys.listFilterAll": "~/.ssh のすべてのファイル",
  "keys.dragKey": "{path} をドラッグ",
  "keys.chosenCount": "{count} 件を選択中",
  "keys.moveTargetLabel": "移動先",
  "keys.moveChosen": "移動",
  "keys.clearChosen": "選択を解除",
  "keys.moveMoved": "{count} 件を移しました。",
  "keys.moveBlocked": "{path} は次の理由で移動できません：{reason}",
  "keys.moveFailed": "{path} を移動できませんでした。",
  "keys.folders": "フォルダ",
  "keys.foldersLabel": "鍵のフォルダ",
  "keys.folderRow": "{name}、{count} 件",
  "keys.folderAll": "すべての鍵",
  "keys.folderUngrouped": "グループなし",
  "keys.relocate": "名前・グループを変更",
  "keys.relocateHeading": "名前・グループの変更：{path}",
  "keys.relocateNote":
    "名前またはグループを変更すると、鍵ファイルが移動します。この鍵を参照する IdentityFile と CertificateFile も同時に書き換えられるため、古いパスへの参照は残りません。",
  "keys.relocateNewName": "名前",
  "keys.relocateGroup": "グループ",
  "keys.relocateSubmit": "鍵を改名・移動",
  "keys.relocateFailed": "鍵を移動できませんでした。",
  "keys.relocateDone": "{path} へ移動しました。",
  "keys.relocateMoved": "移動したファイル",
  "keys.relocateRewritten": "書き換えた設定行",
  "keys.relocateSkipped":
    "鍵名と接尾辞から構成されたファイル名ではないため、次のファイルは移動しませんでした：{paths}",
  "keys.relocateRefused":
    "次の理由により移動できませんでした。ファイルは変更していません：",
  "keys.relocateFilePair": "{from} → {to}",
  "keys.relocateReference": "{directive} {from} → {to} — {path}:{line}",
  "keys.groupNone": "グループなし（~/.ssh 直下）",
  "keys.createGroup": "グループ",
  "keys.blockerTargetOccupied": "{detail} はすでに存在します",
  "keys.blockerUnresolved":
    "{detail} は解決できないパスなので、この鍵を指している可能性があります",
  "keys.blockerReferenceExternal":
    "{detail} は ~/.ssh の外にあるため書き換えられません",
  "keys.blockerGroupNotDeclared":
    "グループ {detail} を宣言する Include 行がありません",
  "keys.blockerDestinationIsConfig":
    "{detail} は Include によって設定ファイルとして読み込まれる場所です",
  "keys.blockerStateDirectory": "{detail} は sshc の状態ディレクトリ内です",
  "keys.blockerOther": "{detail}",
} satisfies Record<MessageKey, string>;
