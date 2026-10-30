# sshcのUI/UX監査と改善案の一覧（フェーズ1）

2026-10-30 JST。対象はmainの`c68affe42`（#508まで）です。監査の時点ではコードを直していません。改善案はこのあと、イシュー #511（まとめ）と #512〜#545（UX-01〜UX-34の順）にしました。直しの進み具合はイシューで追います。

## 結論

- **すぐ直したい不具合が4つあります**。幅の狭いブラウザで同じセッションを開くと、打ったコマンドの行の一部が消えます（UX-25）。Homeは、ふつうの`Host *`に対して「2件の確認が必要です」を出し続けますが、行き先の画面には何も出ません（UX-26）。スマホでは、Menuから開いたページで下のバーのどれも選ばれず（UX-27）、［新しいセッション］を開くとキーボードが開きます（UX-28）。
- **フォームファクタの穴は、タブレット縦とスマホ横です**。タブレット縦（600〜1023pxのタッチ）はスマホの組み方のまま横に伸び、スマホ横は枠が高さを使って中身が約120pxしか残りません（UX-01・UX-02）。2in1は、キーボードを付けた姿勢ではPCと同じで問題がありません。タブレットの姿勢では、PCの組み方のまま20pxの押す場所が残ります（UX-15。方針L57と関わるので判断がほしいです）。
- **機能では、ターミナルの文字の大きさをその場で変えられないことが一番効きます**（UX-16）。Termius・Blinkはピンチで、Windows TerminalはCtrl+= で変えます。sshcはSettingsを開いて［保存］を押すしかありません。
- 拡大とプレビューのアイコン化は#509（別のセッション、作業中）が扱っています。切断の表示は別のセッション（sshc-46）が調べて直しています。どちらも、ここでは新しい案にしていません。

## 1. 進め方

### 1.1 一括撮影のツールの確認

依頼にあった「全画面を一括でスクリーンショットするツール」は`scripts/capture-doc-screenshots.sh`です。そのまま実行して使えることを確かめました。e2e 15件がすべて通り、約4分で27枚を作りました。

ただし、このツールはドキュメントの画像を作るためのもので、今回の目的には足りません。

- 撮るのは、READMEとPagesに載せる27枚だけです。e2eの決まったテストを`--grep`で選んで流します。
- 画面の大きさはPCの1280×720が中心です。スマホは360×800の2枚だけで、タブレットと2in1はありません。
- Settingsの各ページ、SSH Keys、Known Hosts、Diagnostics、Snippets、SSH Configなどは撮りません。
- リポジトリの`docs/images`と`pages/public/images`を上書きします。今回は作業ツリーで流し、画像を写したあとで元に戻しました。

そこで、代わりに撮影のスクリプト`scripts/ux-audit/capture-screens.mjs`を作りました。10-28の点検の`shoot.mjs`を土台にしています。

- 架空のデータだけを入れた一時のHOME（`/large/vm/sshc-ux-audit/ux-home-XXXX`）で、`sshc engine`を127.0.0.1:46420に起動します。バイナリも同じ場所へ写して動かし、プロンプトは`$ `、`ls`は所有者を出さない`ls -F`にしています。イシューに貼る画像に、撮るマシンのユーザー名が写らないようにするためです。終わったら止めてHOMEを消します。
- SSHとSFTPの接続先には、使い捨てのsshdのコンテナ（`scripts/ux-audit/sshd/`、127.0.0.1:2299）を使います。実在のホームディレクトリは映しません。
- 28の画面を4つのフォームファクタで撮り、重い画面は追加の4つの姿勢でも撮ります。ターミナルのプレビュー、メニュー、ダイアログ、SFTPの接続後、接続先の編集のタブ、ロック解除の画面など、操作した状態も撮ります。
- 撮るたびに、横のはみ出し、小さい押す場所、切れた文字、12px未満の文字、主の領域の幅を`manifest*.json`に残します。

画面ごとにフォームファクタを横に並べた画像（`sheets/`・`sheets-extra/`）を作り、すべての画面のものに目を通しました。所見にした画面と、横に並べた画像では細部が読めない画面は、元の大きさの画像も開いて確かめました。

最初の撮影（235枚、`screens-first/`）は手元の一時のフォルダをHOMEにしたため、プロンプトと絶対パスに撮影のマシンのユーザー名が写りました。イシューに登録するにあたり、上の形で撮り直したものを`screens/`に置いています。所見は最初の撮影で見つけ、撮り直しで同じことが起きるかを確かめました。

### 1.2 フォームファクタ

| ID | 用途 | 大きさ（CSS px） | 入力の判定 | 撮った範囲 |
|---|---|---|---|---|
| phone | スマホ縦 | 390×844（DPR 2.625） | タッチだけ | 全画面 |
| tablet | タブレット縦 | 820×1180（DPR 2） | タッチだけ | 全画面 |
| 2in1 | 2in1のノートPCの姿勢 | 1368×912（DPR 2） | 主はマウス（`pointer: fine`）、タッチもある（`any-pointer: coarse`） | 全画面 |
| desktop | デスクトップ | 1920×1080 | マウスだけ | 全画面 |
| phone-landscape | スマホ横 | 844×390 | タッチだけ | Home・Connections・SFTPなど |
| tablet-landscape | タブレット横 | 1180×820 | タッチだけ | 同上 |
| 2in1-tablet | 2in1のタブレットの姿勢 | 1368×912 | タッチだけ（`hover: none`） | 同上 |
| laptop | 小さいノートPC | 1366×768 | マウスだけ | 同上 |

2in1はPlaywrightの`hasTouch`では作れません。`hasTouch`を付けると、主の入力が必ずタッチになり、キーボードを付けた姿勢にならないためです（CDPの`setEmulatedMedia`でも`pointer`は変わりませんでした）。そこでChromiumの起動設定`--blink-settings=primaryPointerType=…`で姿勢を決めました。各画像のブラウザの答え（`pointer`・`hover`・`any-hover`・`any-pointer`）はmanifestに残してあり、どれも意図どおりでした。指のタップはCDPの`Input.dispatchTouchEvent`で送りました。

### 1.3 架空のデータ

接続先18件（グループ3つ、入れ子1つ、長い名前1件、踏み台・転送つき）、セッション5〜6（ワークスペース3つ、接続できないもの1つ）、Vaultの6件、スニペット4件、鍵3つ、known_hosts 4件、sshdの中の写真・動画・ログです。

### 1.4 撮影の制約と、推測の扱い

- **ブラウザはChromiumだけです。** iPadのSafari、AndroidのWebView（APK）、実機の2in1では確かめていません。
- **ライトの配色は確かめていません。** sshcの配色の既定がダークのため、ブラウザをライトにしても画面はダークで写りました（最初の撮影の`light-*`の画像）。
- **ターミナルはWebGLを切って撮りました**。ヘッドレスのChromiumでDPRを2や2.625にすると、WebGLのターミナルが真っ黒に写るか、DPR倍に拡大されて右が切れました。拡大の倍率がちょうどDPRと同じなので、撮影の環境の現象と判断しました（製品の不具合としては扱いません）。e2eと同じ`--disable-webgl`にしています。
- **最初の撮影のHOMEの場所は長いです。** HOMEが一時のフォルダの深い場所だったので、最初の撮影の画像の中の絶対パスはふつうより長く出ています。撮り直した画像のHOMEは`/large/vm/sshc-ux-audit/ux-home-XXXX`です。
- 原因を推測で書いたところには「推定」と付けました。確かめていないことは「未確認」と書きました。

### 1.5 画像の置き場所

画像は撮影の作業ツリーの`artifacts/ux-audit/2026-10-30/`に撮りました。`artifacts/`はgitignoreされているので、リポジトリには入っていません。イシューに貼った撮り直しの画像は、`issue-assets`ブランチの`ux-audit-2026-10-30/`にあります（下の`screens/`と`sheets/`の名前のまま）。

- `screens/<フォームファクタ>--<画面>.png`: 撮り直した画像（イシューに貼ったもの）
- `screens-first/`: 最初の撮影（撮影のマシンのユーザー名が写るので公開しない）
- `sheets/<画面>.png`: phone・tablet・2in1・desktopを横に並べた画像
- `sheets-extra/<画面>.png`: phone-landscape・tablet-landscape・2in1-tablet・laptopを横に並べた画像
- `doc-tool/`: 既存の一括撮影のツールで撮った27枚
- `screens/manifest.json`: 画面ごとの数値
- `notes.md`: 見た順の観察メモ

### 1.6 ほかの作業との関係

| 作業 | 状態 | この一覧での扱い |
|---|---|---|
| プレビューの拡大（ホイール・ダブルクリック・ドラッグ・ピンチ）と見出しのアイコン化 | #509（feat/preview-zoom、別のセッション）で作業中 | 新しい案にしない。#509に含まれない部分だけをUX-17にした |
| 切断の帯とターミナルに書く`[sshc]`の行が同じことを2回言う件 | 別のセッション（sshc-46）が調べて直している | 入れない |
| 10-28の全体点検の担当D（デザインシステム）の所見 | 多くは#483〜#486で直った | 今のmainで残っているものだけを「10-28 Dの再掲」として入れた（付録A） |

## 2. 外部の参考事例

自分で検索して、公式の文書かストアの説明を優先して読みました。二次情報は「二次」と書いています。

| 対象 | 見たこと | 生かせる番号 | 出典 |
|---|---|---|---|
| Termius（スマホ） | 2本の指でつまむと文字の大きさが変わる。長押しのドラッグで矢印キー、ダブルタップでTab。補助キーは4つずつのまとまりで、並べ替えと自作ができる。iPadでは横の欄が右に出る | UX-16、UX-01 | [Mobile terminal](https://docs.termius.com/terminal/mobile-terminal) |
| Termius（スニペット） | ターミナルの横の欄（Ctrl/Cmd+.）からスニペットを「貼る」か「実行」する。開いているすべてのタブで実行もできる | UX-22 | [Snippets](https://docs.termius.com/terminal/snippets) |
| Termius（SFTP） | PCでは左右2つのペインでローカルとリモートを並べる | UX-19 | [Managing files with SFTP](https://docs.termius.com/organize-and-connect-to-hosts/managing-files-with-sftp) |
| Blink Shell（iOS） | ピンチとCmd+／Cmd−で文字の大きさ。2本の指のタップで新しいシェル、スワイプで接続を切り替える | UX-16 | 二次（READMEの写し[yury/blink](https://github.com/yury/blink)）、[ジェスチャーの変更の議論](https://github.com/blinksh/blink/discussions/2010) |
| Prompt 3（Panic） | よく使うコマンドを「Clips」としてキーバーに置き、1回のタップで送る。全体用と接続先ごとに分けられる | UX-22（参考） | [App Store](https://apps.apple.com/app/id1594420480) |
| Secure ShellFish | サーバーのファイルをiOSの「ファイル」アプリに出す。ターミナルからファイル名をドラッグで取り出せる。キーバーのボタンを変えられる | UX-18 | [App Store](https://apps.apple.com/us/app/1336634154) |
| Warp | コマンドと出力を1つの「ブロック」として扱い、⌘↑／↓で移る。⌘Pのパレットで設定も探せる | UX-21・UX-22（sshcは#472でブロックを持つ） | [Blocks](https://docs.warp.dev/terminal/blocks.md)、[Editor](https://docs.warp.dev/terminal/editor) |
| Windows Terminal | Ctrl+Shift+Pのパレット。操作（actions）はすべてキーに割り当てられ、パレットにも出る。文字の大きさはCtrl+= で大きく、Ctrl+0で戻す | UX-16、UX-22 | [Command palette](https://learn.microsoft.com/en-us/windows/terminal/command-palette)、[Actions](https://learn.microsoft.com/en-us/windows/terminal/customize-settings/actions)、[#2578](https://www.github.com/microsoft/terminal/issues/2578) |
| Tabby | 入れ子の分割、ペインのドラッグでの並べ替え、SFTPの欄を今のフォルダで開く | UX-19 | [README](https://github.com/Eugeny/tabby) |
| Linear | Cmd+Kでその画面でできる操作を名前で探す。キーボードだけで動かせる | UX-21 | 二次（[DEVの解説](https://dev.to/jessyt/linear-the-keyboard-first-issue-tracker-teams-pick-over-jira-4m6e)） |
| Material 3（画面幅の区分） | 600dp未満がcompact、600〜839dpがmedium（タブレット縦）、840dp以上がexpanded。medium以上ではナビゲーションレールを使い、compactでは使わない | UX-01、UX-02 | [Window size classes](https://developer.android.com/develop/ui/compose/layouts/adaptive/window-size-classes)、[Large screens](https://developer.android.com/design/ui/large-screens) |
| Apple HIG | タッチの押す場所は44×44pt以上。下のバーは、どの区画にいるかを忘れさせないために表示し続ける | UX-15、UX-27 | [Buttons](https://developer.apple.com/design/human-interface-guidelines/buttons)、Tab barsは公式の写し（[unpkgの写し](https://unpkg.com/apple-hig-mcp@1.1.3/content/tab-bars.md)）で確認 |
| Microsoft（Windowsのアプリ） | タッチの押す場所は40×40 epx、詰めた表示でも32×32 epx | UX-15 | [Guidelines for touch targets](https://learn.microsoft.com/en-us/windows/apps/design/input/guidelines-for-targeting) |
| WCAG 2.2（2.5.8） | 押す場所は24×24 CSS px以上（隣と24px以上離れていれば例外） | UX-15 | 二次（[DAISYの解説](https://kb.daisy.org/publishing/docs/wcag/target-size-minimum.html)） |
| 写真の表示（iOSの写真など） | ダブルタップとピンチで触った所を中心に拡大。拡大中は端まで動かしてから次の写真へ。拡大していないときは下へのドラッグで閉じる | UX-17 | 二次（[photo_zoomの説明](https://pub.dev/packages/photo_zoom)）。Apple・Googleの公式の説明は見つからなかった |

## 3. 一覧

番号は後で並べ替えても変えません。優先度は「高・中・低」、規模は「小（1日以内）・中（数日）・大（1週間以上、設計から）」の目安です。

| 番号 | 分類 | 内容 | 対象 | フォームファクタ | 優先度 | 規模 |
|---|---|---|---|---|---|---|
| UX-01 | デザイン | タブレット縦がスマホの組み方のまま横に伸びる | 全体の枠 | tablet | 中 | 大 |
| UX-02 | デザイン | スマホ横で枠が高さを使い、中身が約120pxしか残らない | 全体の枠・SFTP | phone-landscape | 中 | 中 |
| UX-03 | デザイン | 一覧＋詳細のページで、選ぶ前は詳細の大きな領域が空き、一覧の列は狭くて名前が切れる | Connections・Snippets | 2in1・desktop・tablet | 中 | 中 |
| UX-04 | デザイン | 広い画面で板が中央の最大幅に収まり、左右が空く。Settingsのナビが空きに浮く | Home・Settingsほか | desktop | 低 | 小 |
| UX-05 | デザイン | SSH ConfigのIncludeの木でファイル名が切れ、パターンが単語の途中で折り返す | SSH Config | 2in1・desktop | 低 | 小 |
| UX-06 | デザイン | SFTPの一覧: スマホは権限とUIDが先、PCは7列が離れて読みにくい。画像・動画の区別が無い | SFTP | 全部 | 中 | 小 |
| UX-07 | デザイン | ファイルのプレビューの枠: 画面の中の小さなダイアログで、画像が上に寄り下が空く。SFTPの詳細はボタンが2段 | SFTP・ターミナルのプレビュー | phone・tablet | 中 | 中 |
| UX-08 | デザイン | Diagnostics: 接続名が自由入力で、例の文字が値に見える。設定ファイルが絶対パス | Diagnostics | 全部 | 低 | 小 |
| UX-09 | デザイン | Homeのカード: 「グループなし」の札、高さのばらつき、検索の例の文字が切れる | Home | phone・tablet | 低 | 小 |
| UX-10 | デザイン | Settings > Terminalの設定の組の帯が、スマホで3段に折り返す | Settings > Terminal | phone | 低 | 小 |
| UX-11 | デザイン | SSH Keys: 作成の入口が二重、絞り込みの部品が3つ、ssh-agentの警告が両方のOSの手順を出す | SSH Keys | phoneほか | 低 | 小 |
| UX-12 | デザイン | Known Hostsのスマホのカードが1件4行で長い | Known Hosts | phone | 低 | 小 |
| UX-13 | デザイン | 接続先の編集のタブがスマホで横にはみ出し、続きがある印が無い | Connectionsの編集 | phone | 低 | 小 |
| UX-14 | デザイン | サイドバーの接続できなかったセッションの2行目が「接続できませんでした。理由はター…」と切れる | サイドバー | 全部 | 低 | 小 |
| UX-15 | デザイン | タッチだけの大きい画面（タブレット横・2in1のタブレットの姿勢）で、20pxの押す場所が残る | サイドバー・一覧の行 | tablet-landscape・2in1-tablet | 中（要判断） | 中 |
| UX-16 | 機能 | ターミナルの文字の大きさを、その場で変えられるようにする | ターミナル | 全部 | 高 | 中 |
| UX-17 | 機能 | プレビューの指の操作（左右で前後、下で閉じる）、スマホの全画面、SFTPの詳細の前へ・次へ | プレビュー | phone・tablet | 中 | 中 |
| UX-18 | 機能 | SFTPの画像・動画のサムネイルと、並べて見る表示 | SFTP | 全部 | 低 | 大 |
| UX-19 | 機能 | SFTPを1回の操作で左右2つのペイン（ローカル＋リモート）にする | SFTP | 2in1・desktop | 中 | 小 |
| UX-20 | 機能 | SFTPの空の状態に、最近の接続先を直接並べる | SFTP | 全部 | 中 | 小 |
| UX-21 | 機能 | スマホのMenuの上に検索を置く | Menu | phone・tablet | 低 | 小 |
| UX-22 | 機能 | キーボードショートカットに割り当てられる操作を増やす | Settings > Keyboard shortcuts | 2in1・desktop | 中 | 中 |
| UX-23 | 機能 | Homeのカードにも長押し（右クリック）のメニューを付ける | Home | 2in1・tablet-landscape | 低 | 小 |
| UX-24 | 機能 | 常に出る長い説明を畳む（10-28 Dの再掲を含む） | Vaultの作成・Sync | 全部 | 低 | 小 |
| UX-25 | 不具合 | 幅の狭いブラウザで同じセッションを開くと、打ったコマンドの行の一部が消える | ターミナル | 全部 | 高 | 中 |
| UX-26 | 不具合 | Homeが、ふつうの`Host *`に対して「2件の確認が必要です」を出し続ける。行き先には何も出ない | Home | 全部 | 中 | 小 |
| UX-27 | 不具合 | スマホの下のバーで、Menuから開いたページのときにどれも選ばれない | 下のバー | phone・tablet | 中 | 小 |
| UX-28 | 不具合 | スマホで［新しいセッション］と実行先の選択を開くと、検索欄にフォーカスが入りキーボードが開く | 新しいセッション | phone・tablet | 中 | 小 |
| UX-29 | 不具合 | Diagnosticsの設定ファイルの一覧で、状態の点が潰れ、長いパスが右で切れる | Diagnostics | phone | 低 | 小 |
| UX-30 | 不具合 | Homeの確認の帯のボタンが文の右にある（方針L10）。共通の知らせの部品を使っていない | Home | 全部 | 低 | 小 |
| UX-31 | 不具合 | 復元の知らせに履歴IDが出る（10-28 Dの再掲） | History | 全部 | 低 | 小 |
| UX-32 | デザイン | 設定の確定の仕方がページごとに違う（すぐ保存と［保存］が混ざる） | Settings | 全部 | 中 | 小 |
| UX-33 | 機能 | 検索（パレット）の結果を、一致の度合いで並べる | 検索 | 2in1・desktop | 低 | 小 |
| UX-34 | 不具合 | 動画・音声のプレビューで、ファイルが無いときも「このブラウザでは再生できない形式です」と出る | プレビュー | 全部 | 低 | 小 |

## 4. デザイン改善案

### UX-01 タブレット縦がスマホの組み方のまま横に伸びる

- **対象**: 全体の枠（Home、ターミナル、SFTPなど）。tablet（820×1180）。2in1を縦にしたタブレットの姿勢（912×1368、`pointer: coarse`）も、下の条件から同じ組み方になる（縦の姿勢は撮っていない）。
- **今の問題**: 820pxの幅があるのに、上のバー・下のバー・引き出しのスマホの組み方で出ます。下のバーは幅いっぱいに伸び、5つのアイコンの間が約150px空きます。セッションの一覧は引き出しの中にしか無く、ターミナルを見ながら別のセッションへ移るには毎回引き出しを開きます。ターミナルの補助キーと入力欄も820pxに引き伸ばされます。
  - `screens/tablet--home.png`、`screens/tablet--drawer-open.png`、`screens/tablet--terminal-local-session.png`
- **原因**: 組み方の切り替えは`mobileViewportQuery`（`(max-width: 767px), (pointer: coarse) and (max-width: 1023px)`、`web/src/ui/useMediaQuery.ts:6`）の1本だけで、「スマホ」と「PC」の2通りしかありません。600〜1023pxのタッチの画面にあたる中間の組み方がありません。
- **改善案**: タッチで600〜1023pxのときの組み方を足します。
  - A: 左にアイコンだけの細いレール（Home・Connections・SFTP・ターミナル・Menu）を常に出し、下のバーをやめる。セッションの一覧は今の引き出しのまま。
  - B: 横にしたPCのサイドバー（幅を詰めた形）を常に出し、ターミナルの右に中身を置く。820pxなら260＋560px。
  - 参考: Material 3は600〜839dpをmediumとし、ナビゲーションレールを勧めています。TermiusはiPadで横の欄を右に出します。
- **優先度・規模**: 中・大（組み方を1つ足すので、全画面の確認が要る）。

### UX-02 スマホ横で枠が高さを使い、中身が約120pxしか残らない

- **対象**: 全体の枠。SFTPが最も重い。phone-landscape（844×390）。
- **今の問題**: SFTPでは、上のバー・タブ・パスの帯・転送の帯・下のバーで約270pxを使い、中身が約120pxしか残りません。空の表示の見出しと［接続先を選択］が切れて見えません。Homeも1段のカードの半分が下のバーに隠れます。
  - `screens/phone-landscape--files.png`、`screens/phone-landscape--home.png`、`sheets-extra/home.png`
  - 2026-10-10の記録（sshcのすすめ「撮影結果と横向きで見つかった表示制約」）で既に指摘され、直していないままです。
- **原因**: 縦の組み方の帯を、高さが390pxのときもそのまま積んでいます。高さで組み方を変える条件がありません。
- **改善案**: 高さが小さい（たとえば480px未満）ときは、下のバーを左の細いレールに移し、上のバーとタブを1段にまとめます。転送の帯は件数が0のときは出しません。参考: Material 3は幅だけでなく高さにも区分（480dp未満がcompact）を持ちます。横向きのスマホでレールに移す形は、Materialのmedium以上でレールを使う考え方からの私の案です（Materialが横向きのスマホについてそう書いているかは確かめていない）。
- **優先度・規模**: 中・中。

### UX-03 一覧＋詳細のページで、選ぶ前は詳細の大きな領域が空き、一覧の列は狭くて名前が切れる

- **対象**: Connections、Snippets。2in1・desktop・tablet-landscape。
- **今の問題**: 1920幅のConnectionsは、サイドバー250px、グループ235px、一覧 約300px、詳細 約1120pxの4列です。何も選んでいない間、詳細の1120pxには「接続先を選択」と2つ目の［接続先を追加］しかありません。一方で一覧の列は300pxのままで、`analytics-batch-worker-very-lon…`と名前が切れます。Snippetsも同じで、コマンドが`sudo systemctl restart {{service…`と切れ、説明は一覧に出ません。
  - `screens/desktop--connections.png`、`sheets/connections.png`、`sheets/snippets.png`
- **原因**: 一覧の列の幅を固定にし、詳細の領域は「選んだものを編集する場所」としてしか使っていません。選ぶ前の状態を設計していません（推定）。
- **改善案**:
  - A: 何も選んでいないときは、詳細の領域を一覧の表にする。列は名前・`user@host:port`・グループ・踏み台・最終接続。選ぶと今の詳細に替わる。
  - B: 一覧の列の幅を変えられるようにし、広い画面では既定を広くする。
  - 参考: TermiusはPCで接続先を表のように並べ、編集は横の欄で開きます。Material 3の「一覧と詳細」の型も、広い画面では一覧を広く取ります。
- **優先度・規模**: 中・中。

### UX-04 広い画面で板が中央の最大幅に収まり、左右が空く

- **対象**: Home、Settings、Menuから開くページ。desktop（1920×1080）。
- **今の問題**: Homeの板は約1135pxで中央に置かれ、サイドバーとの間に約270px、右に約270pxの空きがあります。カードは3列のままです。Settingsでは、空きの中にSettingsの小さいナビ（200px）が浮きます。
  - `screens/desktop--home.png`、`screens/desktop--settings-shortcuts.png`
- **原因**: 板の最大幅を決め、中央に置く作りです。読みやすい1行の長さを保つための決めで、それ自体は正しいです。ただ、カードの一覧のように横に増やせる中身も同じ最大幅に収めています（推定）。
- **改善案**: カードや表のページは最大幅を外し、列を増やします。文章や入力のページ（Settings）は今の最大幅のまま、板をSettingsのナビに寄せて置きます。
- **優先度・規模**: 低・小。

### UX-05 SSH ConfigのIncludeの木でファイル名が切れる

- **対象**: SSH Config。2in1・desktop。
- **今の問題**: 木の列（約270px）で`connections/production/o…`のように切れます。Includeのパターンの行は`connections/production/osak a/*.conf`と単語の途中で折り返します。
  - `screens/desktop--config.png`
- **原因**: 木の階層で場所が分かるのに、各行が`~/.ssh`からの相対パスを全部出しています。
- **改善案**: ファイルの行はファイル名（`osaka.conf`）だけにし、パスは載せたときのツールチップに出します。列の幅を変えられるようにします。
- **優先度・規模**: 低・小。

### UX-06 SFTPの一覧が読みにくい

- **対象**: SFTP。全部。
- **今の問題**:
  - スマホの2行目が`-rw-r--r--  1000:1000  27.3 KiB  2026/10/30`で、権限とUID:GIDが先に来ます。
  - PC（1920）は1つのペインのまま1650pxの表に7列で、名前の列400pxのあとに列が離れて並びます。1行を読むのに目が横に大きく動きます。
  - 画像・動画も「ファイル」で、アイコンも同じです。
  - `screens/phone--sftp-remote-photos.png`、`screens/desktop--sftp-remote-photos.png`
- **原因**: PCの表の列をスマホの1行へそのまま詰め、列の幅を表の全幅に配っています（推定）。
- **改善案**: スマホは「大きさ・更新日時」を先にし、権限と所有者は詳細に回します。PCは名前の列だけを伸ばし、ほかの列は中身の幅にします。種類（画像・動画・文書・圧縮）ごとのアイコンにします。参考: FinderとiOSの「ファイル」のリスト表示は、名前・日付・大きさ・種類を並べ、権限は情報の欄に置きます。
- **優先度・規模**: 中・小。

### UX-07 ファイルのプレビューの枠

- **対象**: SFTPの詳細のダイアログ、ターミナルのプレビュー。phone・tablet。
- **今の問題**:
  - どちらも画面の中のダイアログ（周りに余白と暗い背景）で、画像は幅に縮められて上に寄り、下の約半分が空きます。
  - SFTPの詳細は、下に［ファイルを編集］［ダウンロード］／［所有者・グループを変更］［名前変更］が2段で右寄せに並びます。JPEGにも［ファイルを編集］が出ます（押すと何が開くかは未確認）。
  - `screens/phone--sftp-preview-image.png`、`screens/phone--terminal-preview-image.png`
- **原因**: プレビューを、PCの大きさのダイアログの部品でスマホにも出しています。
- **改善案**: スマホでは全画面にし、画像は縦の中央に置きます。SFTPの詳細の操作は「…」にまとめ、［ダウンロード］だけを見出しに出します。テキストでないファイルには［ファイルを編集］を出しません。拡大の操作と見出しのアイコンは#509に任せます。
- **優先度・規模**: 中・中。

### UX-08 Diagnosticsの入力と結果の見せ方

- **対象**: Diagnostics。全部（スマホが最も重い）。
- **今の問題**: 接続名は自由入力で、例の`bastion`が値のように見えます。保存した18件から選ぶ部品がありません。設定ファイルの一覧は絶対パスで、スマホでは1件が4行になります（ふつうの`/home/<ユーザー>/.ssh/connections/<グループ>/<ファイル>.conf`でも2行）。
  - `screens/phone--diagnostics.png`、`sheets/diagnostics.png`
- **原因**: 入力を文字の欄だけで作り、パスはsshcエンジンが返した値をそのまま出しています。
- **改善案**: 接続名は、保存した接続先から選べる入力（打つと候補が出る）にします。パスは`~/.ssh`からの相対で出します（10-28 Dの「パスは~/.sshからの相対で出し」と同じ方針）。
- **優先度・規模**: 低・小。点が潰れる・パスが切れる件は不具合としてUX-29に分けました。

### UX-09 Homeのカード

- **対象**: Home。phone・tablet。
- **今の問題**: 右に「グループなし」の札が出ますが、情報がありません。接続したことのある2件だけ「最終接続」の行があり、カードの高さがそろいません。スマホのカードは1枚約89pxで、Connectionsの行（約50px）の倍近く、1画面に5〜6件です。検索の例の文字が「名前やアドレスで絞り込」で切れます。
  - `screens/phone--home.png`、`screens/tablet--home.png`
- **原因**: カードに出す項目を、値があるか・意味があるかで出し分けていません。検索欄の右の［パネル｜リスト］が幅の約3分の1を取ります。
- **改善案**: 「グループなし」の札は出しません。最終接続は札（「3日前」など）にして高さをそろえます。スマホでは［パネル｜リスト］を検索欄の下の小さい切り替えにするか、「…」に入れます。
- **優先度・規模**: 低・小。

### UX-10 Settings > Terminalの設定の組の帯がスマホで3段に折り返す

- **対象**: Settings > Terminal。phone。
- **今の問題**: 1段目に選択、2段目に「このブラウザで使用中」［新しい組］［名前を変更］、3段目に［削除］だけが並びます。
  - `screens/phone--settings-terminal.png`（PCの形は`screens/2in1--settings-terminal.png`）
- **原因**: PCの1行の帯を、スマホで折り返すに任せています。
- **改善案**: スマホでは［新しい組］［名前を変更］［削除］を「…」にまとめ、選択と札を1行にします。
- **優先度・規模**: 低・小。

### UX-11 SSH Keysの入口と警告

- **対象**: SSH Keys。phoneほか。
- **今の問題**: 見出しの［＋鍵を作成］と、ページの下の「鍵を作成」の節が二重です。鍵3件に対して、絞り込みの部品が3つ（鍵だけ／検索／すべての鍵(3)）あります。ssh-agentの警告は6行で、macOS・LinuxとWindowsの両方の手順を同時に出します。
  - `screens/phone--keys.png`
- **原因**: 作成の節をページに常に置いたまま、見出しにも入口を足しています（推定）。警告の文はOSを見ていません。
- **改善案**: 作成は見出しのボタンから開く欄だけにします。件数が少ないときは絞り込みを畳みます。警告は今のOSの手順だけを出します。
- **優先度・規模**: 低・小。

### UX-12 Known Hostsのスマホのカード

- **対象**: Known Hosts。phone。
- **今の問題**: 1件を「ホスト／鍵種別／フィンガープリント／削除」の4行のカード（約150px）で出し、4件で2画面になります。ハッシュ化したエントリは「（ハッシュ済み）」だけで、どの接続先のものか分かりません。
  - `sheets/known-hosts.png`
- **改善案**: 1件を2行（ホストと鍵種別、フィンガープリントの先頭）にし、タップで詳細を開きます。ハッシュ化したエントリは、保存した接続先のHostNameと照らして名前を出します（できるかは未確認）。
- **優先度・規模**: 低・小。

### UX-13 接続先の編集のタブがスマホで横にはみ出す

- **対象**: Connectionsの接続先の編集。phone。
- **今の問題**: 4つのタブ（基本／踏み台・転送・その他／表示とターミナル／最終的な値）が横にはみ出し、4つ目が「最終」で切れます。横にスクロールできますが、続きがある印がありません。
  - `screens/phone--connection-detail-basic.png`
- **改善案**: 端をぼかして続きを示すか、スマホではタブを選択の部品にします。
- **優先度・規模**: 低・小。

### UX-14 サイドバーの接続できなかったセッションの2行目が切れる

- **対象**: サイドバーのセッションの行。全部。
- **今の問題**: 2行目が「接続できませんでした。理由はター…」と切れ、続きが読めません。
  - `screens/2in1--home.png`、`screens/desktop--home.png`
- **原因**: 行に入らない長さの文を、そのまま2行目に入れています。
- **改善案**: 2行目は「接続できません」などの短い状態にし、理由はターミナルの領域とツールチップに出します。
- **優先度・規模**: 低・小。

### UX-15 タッチだけの大きい画面で、20pxの押す場所が残る

- **対象**: サイドバーのセッションの行の「…」「×」、一覧の行の操作。tablet-landscape（1180×820）、2in1-tablet（1368×912、`hover: none`）。
- **今の問題**: どちらもPCの組み方で出ます。manifestで数えると、撮った6画面（Home・Menu・Connections・SFTP・Settings > Terminal・ターミナル）に見えている押す場所のうち、24px未満が合わせて117個ありました（どちらの姿勢も同じ数）。多いのはサイドバーの「…」と「×」（20×20px）です。
  - `screens/manifest.json`（tablet-landscape・2in1-tabletの`smallTargets`）、`sheets-extra/home.png`
- **原因**: 方針L57により、タッチ向けの見た目はスマホの組み方のときだけにしています。この方針は「タッチ画面付きのPCでも画面がPCの大きさならPCと同じにする」という相談で決めたもので、キーボードもマウスも無い姿勢（タブレット横、2in1のタブレットの姿勢）は話題に出ていません。
- **改善案**: `(any-hover: none)`かつPCの組み方のときだけ、行の操作を32〜40pxにします（Microsoftは40 epx、詰めた表示で32 epx。Appleは44pt）。マウスのある2in1は今のままです。
- **方針との関係**: L57の範囲を変える話なので、進めてよいかを決めてください。
- **優先度・規模**: 中（要判断）・中。

### UX-32 設定の確定の仕方がページごとに違う

- **対象**: Settingsの各ページ。全部。
- **今の問題**: Appearance・Notifications・SFTPは変えるとすぐに保存し（SFTPの見出しに「変えるとすぐに保存します。」とある）、Engine・Terminalは下の［保存］で確定します。SFTPのページの中でも、転送の除外だけは［除外指定を保存］を押します。どのページで［保存］が要るかを、見た目から予想できません。
  - `sheets/settings-appearance.png`、`sheets/settings-engine.png`、`sheets/settings-sftp.png`、`sheets/settings-notifications.png`、`screens/2in1--settings-terminal.png`
- **原因**: ページごとに作った時期と作り方が違い、確定の仕方の決まりがありません（推定）。
- **改善案**: どちらかにそろえます。すぐ保存にそろえるなら、保存したことを入力の横に小さく出し、Engineの待受ポートのように再起動が要るものだけ確認を挟みます。［保存］にそろえるなら、変更があるときだけ下に保存の帯を出します。方針L8（済んだ結果を出し続けない）とも合わせます。
- **優先度・規模**: 中・小。

## 5. 機能追加

### UX-16 ターミナルの文字の大きさを、その場で変えられるようにする

- **対象**: ターミナル。全部。
- **今の問題**: 文字の大きさを変える道はSettings > Terminalだけで、欄に数字を入れて［保存］を押します。ターミナルの上でピンチ・Ctrl＋ホイール・Ctrl+=／−／0のどれも効きません（ホイールの処理はスクロールだけ）。ショートカットの一覧にもありません。スマホでPCのセッションを開いたときや、人に画面を見せるときに困ります。
  - `screens/2in1--settings-terminal.png`、`screens/desktop--settings-shortcuts.png`、`web/src/rules/shortcuts.generated.ts:16`
- **改善案**: ピンチ（タッチ）、Ctrl＋ホイール、Ctrl+=／Ctrl+−／Ctrl+0（Macは ⌘）で、表示中のターミナルの文字の大きさを変えます。変えた値はこのブラウザの設定の組（#467）に入れるか、セッションだけに効かせるかを選べるようにします。参考: Termiusのピンチ、BlinkのピンチとCmd+／Cmd−、Windows TerminalのCtrl+= とCtrl+0。
- **注意**: Ctrl＋ホイールはブラウザのページの拡大と取り合います。ターミナルの上だけで取ります。
- **優先度・規模**: 高・中。

### UX-17 プレビューの指の操作とスマホの全画面

- **対象**: ターミナルのプレビュー、SFTPの詳細のプレビュー。phone・tablet。
- **今の問題**: #509で拡大（ピンチを含む）とアイコン化は入りますが、次のものはありません。
  - 左右のスワイプで前後のファイルへ移る（今は［前へ］［次へ］のボタンだけ）
  - 拡大していないときの下へのスワイプで閉じる
  - SFTPの詳細のプレビューに前へ・次へ（同じフォルダの次の画像へ移れない）
  - `screens/phone--terminal-preview-image.png`、`screens/phone--sftp-preview-image.png`
- **改善案**: 写真のアプリと同じ操作にします。拡大中のドラッグは画像を動かし、端まで来たら次の画像へ移ります。全画面の枠はUX-07と一緒に作ります。
- **優先度・規模**: 中・中（#509のあと）。

### UX-18 SFTPの画像・動画のサムネイル

- **対象**: SFTP。全部。
- **今の問題**: 写真のフォルダでも、開くまで中身が分かりません（`screens/phone--sftp-remote-photos.png`）。
- **改善案**: 画像・動画の行に小さいサムネイルを出し、写真のフォルダでは並べて見る表示（グリッド）に切り替えられるようにします。サムネイルはsshcエンジンで縮めて作り、大きいファイル・多いフォルダでは作らない上限を設けます。参考: iOSの「ファイル」のアイコン表示、Secure ShellFishの「ファイル」アプリとの連携。
- **優先度・規模**: 低・大（sshcエンジンで縮める処理と、転送の量の扱いが要る）。

### UX-19 SFTPを1回の操作で左右2つのペインにする

- **対象**: SFTP。2in1・desktop。
- **今の問題**: 1920幅でも1つのペインで始まります（`screens/desktop--sftp-remote-photos.png`）。左右2つにするには［＋］で2つ目のタブを開き、接続先を選び、タブの「…」から［右に置く］を選ぶ（またはタブを右半分へドラッグする）必要があります（`web/src/sftp/SFTPWorkspace.tsx:274-289`）。
- **改善案**: ツールバーに［左右に並べる］を置き、押すと右にローカル（または最後に使った接続先）のペインを開きます。最後の配置を覚えて、次に開くときもその形にします。参考: Termius・WinSCP・FileZillaは最初から左右2つです。
- **優先度・規模**: 中・小。

### UX-20 SFTPの空の状態に、最近の接続先を直接並べる

- **対象**: SFTP。全部。
- **今の問題**: 空の状態は［接続先を選択］→ ダイアログ → 選ぶ、の2回の操作です。未接続でもパスの帯（↑ / ∨ 検索 …）と「転送0件」の帯が出ます（`screens/phone--files.png`、`sheets/files.png`）。
- **改善案**: 空の状態の中に「ローカル」と最近の接続先を3〜5件並べ、1回で開けるようにします。未接続のときはパスの帯を出しません。
- **優先度・規模**: 中・小。

### UX-21 スマホのMenuの上に検索を置く

- **対象**: Menu。phone・tablet。
- **今の問題**: Menuは22項目で、スマホでは約3画面ぶんスクロールします（`screens/phone--menu.png`）。PCの検索（Ctrl+K）にあたる入口は、スマホでは引き出しの中にあります。
- **改善案**: Menuの上に検索の欄を置き、押すと今の検索（パレット）を開きます。ページの名前だけでなく、設定の項目（「文字の大きさ」など）でも引けるようにします。参考: iOSの設定の検索、Windows Terminal・Warpのパレットで設定を探す使い方。
- **優先度・規模**: 低・小（設定の項目まで引くなら中）。

### UX-22 キーボードショートカットに割り当てられる操作を増やす

- **対象**: Settings > Keyboard shortcuts。2in1・desktop。
- **今の問題**: 割り当てられる操作は10種（コマンド検索、ターミナル内検索、コピー、貼り付け、前後のプロンプト、前後のセッション、Homeを開く、SFTPを開く）です。新しいセッション、セッションを閉じる、分割、分割の中の移動、サイドバーの開閉、文字の大きさ、クイックコマンドはありません。
  - `screens/desktop--settings-shortcuts.png`、`web/src/rules/shortcuts.generated.ts:16`
- **改善案**: 上の操作を足し、既定のキーはブラウザと取り合わないものにします（Ctrl+Shift+Tなどはブラウザが先に取る）。参考: Windows Terminalはすべての操作を割り当てられ、パレットにも出ます。Termiusはスニペットを「すべてのタブで実行」できます。
- **優先度・規模**: 中・中。

### UX-23 Homeのカードにも長押し（右クリック）のメニューを付ける

- **対象**: Home。2in1・tablet-landscape。
- **今の問題**: Homeのカードの「…」は、マウスを載せられる画面では載せたときだけ出ます（`revealOnHover`、`web/src/overview/QuickConnectBrowser.tsx:182`）。2in1のノートPCの姿勢で指でタップすると、そのまま接続してターミナルへ移ります（`screens/2in1--touch-home-tap.png`）。サイドバーの行（`web/src/terminal/SessionList.tsx:246`）とConnectionsの一覧には長押しのメニューがあり、Homeのカードにだけありません。
- **改善案**: Homeのカードにも長押しと右クリックで同じメニューを出します。
- **優先度・規模**: 低・小。

### UX-24 常に出る長い説明を畳む

- **対象**: Vaultの作成・ロック解除の画面、Sync。全部。
- **今の問題**: 初めてVaultを作る画面に、赤い「このブラウザからサインアウト」と2文の説明が常に出ます（10-28 Dの再掲）。Syncの未設定の画面は、同期の対象と安全についての4行（スマホでは7行）が常に出ます。
  - `sheets/entry-create-vault.png`、`sheets/entry-unlock-vault.png`、`sheets/sync.png`
- **改善案**: サインアウトは「…」か小さいリンクにし、説明は開いたときだけ出します。Syncの説明は知らないと困る中身なので消さずに、1行の要約と［詳しく］にします（方針L10・L32）。
- **優先度・規模**: 低・小。

### UX-33 検索（パレット）の結果を、一致の度合いで並べる

- **対象**: 検索（Ctrl+K）。2in1・desktop。
- **今の問題**: 「web」と打つと、`osaka-web-01`・`osaka-web-02`が先に出て、名前が「web」で始まる`web-01`は10番目です。開いているセッションの`web-01`は、さらにその下です。
  - `sheets/command-palette.png`
- **原因**: 打った語を含むかどうかで絞り込むだけで（`web/src/shell/CommandPalette.tsx:49-52,176`）、並びは種類の順（操作→接続先→セッション→ファイル）と設定ファイルの中の順のままです。
- **改善案**: 種類の順は今のまま、同じ種類の中を「名前が同じ」「名前が打った語で始まる」「単語の頭で一致」「途中で一致」の順に並べます。最近使ったものを少し上げることも考えます。参考: LinearやRaycastの検索は、一致の度合いと最近の使用で並べます（二次情報）。
- **優先度・規模**: 低・小。

## 6. 既存機能の修正・不具合

### UX-25 幅の狭いブラウザで同じセッションを開くと、打ったコマンドの行の一部が消える

- **対象**: ターミナル。phone・tablet・laptop（1366）で再現。
- **今の問題**: 1920幅のブラウザで画面から開いたローカルシェルに、印を付けた長いコマンド（`echo _00_abcdefghij_01_…_23_abcdefghij`）を2回打ちました。打った側では正しく折り返されます。そのセッションを1366幅・820幅・390幅のブラウザで開くと、コマンドの行のうち、1920幅での1行目の末尾にあたる部分（この撮影では`_10_`〜`_14_`。プロンプトの長さで変わる）が消えます。この再現では、出力の行は正しく折り返し直されました。打った側の表示は、ほかのブラウザで開いたあとも正しいままです。
  - 崩れた行は文字が混ざったまま画面に残り、パスとして押せます。撮り直しの2in1（1368幅）では、出力の1行目が`…/Pictures/IMG_2041.jpgcording.mp4`になり（`IMG_2041.jpg`の後ろに、別の行の`demo-recording.mp4`の末尾が残った）、押すと存在しないファイルのプレビューが開きました（`screens/2in1--terminal-image-session.png`、`screens/2in1--terminal-preview-image.png`）。
  - 別の条件では出力の行も欠けました。APIで120桁を指定して作ったセッションに1920幅のブラウザで打った場合で、開き直すたびに崩れ方が変わりました（下の2枚）。
  - 打った側: `screens/desktop--reflow-1-typed.png`、`screens/desktop--reflow-3-typed-after.png`
  - 後から開いた側: `screens/laptop--reflow-2-other.png`、`screens/tablet--reflow-2-other.png`、`screens/phone--reflow-2-other.png`
  - 同じセッションでも開くたびに崩れ方が変わる例（最初の撮影。撮影のマシンのユーザー名が写るので公開していない）: `screens-first/desktop--terminal-local-session.png`（3つ目の出力が`…/s`で途切れる）と`screens-first/desktop--terminal-more-actions.png`（3つ目の出力が無い）
- **原因（推定）**: 作り直したターミナルは、sshcエンジンが保存した出力のバイト列を、今のブラウザの幅で流し直します。#501の画面の状態を使うのは出力が保存の上限（256KiB）を超えたセッションだけで、ふつうのセッションはバイト列のままです（worklog 2026-10-29 16:49）。シェルの行の編集（readline）は、打った時点の幅を前提にカーソルを動かします。そのため、違う幅で流し直すと、行の途中の文字が別の場所に書かれたり、消えたりします。画面から開いたセッションの再現で出力の行が崩れなかったのは、出力がカーソルを動かさずに右端で折り返すだけだったためと考えます。大きさが途中で変わったセッション（上の別の条件）では出力の行も欠けたので、行の編集だけの問題とは言い切れません。
- **改善案**:
  - A: 流し直すときは、そのバイト列を書いた時の幅でターミナルを作って書き込み、書き終えてから今の幅へ変えます。xterm.jsが折り返しを直します。sshcエンジンは大きさの変更を（書いた位置, 大きさ）で既に積んでいます（#501）。
  - B: 大きさが違うときは、いつも#501の画面の状態（sshcエンジンで折り返し直したもの）から作ります。tmuxはこの方式で、ペインごとに画面を持ち、大きさが変わると折り返し直します。
  - どちらの案でも、`web/src/terminal/screenSnapshot.test.ts`の場面に「幅の境で行を編集したシェルの行を、狭い幅で開く」を足します。
- **優先度・規模**: 高・中（Aなら中、Bなら大）。スマホでPCのセッションを開く使い方（方針L101「tmuxのようになってほしい」）で毎回起きうるためです。

### UX-26 Homeが、ふつうの`Host *`に対して「2件の確認が必要です」を出し続ける

- **対象**: Home。全部。
- **今の問題**: `~/.ssh/config`の最後に`Host *`と`ServerAliveInterval 30`を書いただけで、Homeに「設定または復旧について2件の確認が必要です」［設定を確認］が出続けます。2件の中身は`unnamed_host_block`と`wildcard_shadow`（どちらもconfig:26の`Host *`）です。［設定を確認］の行き先のSSH Configは「診断0件」「Includeの問題はありません」と出し、何を確認すればよいか分かりません。
  - `screens/2in1--home.png`、`screens/desktop--config.png`、`screens/setup.json`の`attention`
- **原因**: この2つはConnectionsでは「接続先を選んだときの注意」（`selectionNoticeCodes`、`web/src/connections/ConnectionsPage.tsx:77`）として扱うものです。Homeは数えない種類に`group_empty`だけを入れていて（`web/src/overview/OverviewPanel.tsx:32`）、この2つも「確認が必要」と数えます（同`:98-103`）。
- **改善案**: Homeが数えるのは、利用者が直すべき問題（読めないファイル、壊れたInclude、復旧待ちなど）だけにします。どれを数えるかをConnectionsと同じ表から決め、帯の文には「何について・どこで」を入れます（方針L17）。
- **優先度・規模**: 中・小。

### UX-27 スマホの下のバーで、Menuから開いたページのときにどれも選ばれない

- **対象**: 下のバー。phone・tablet（縦）。
- **今の問題**: MenuからSSH KeysやSettings > Terminalを開くと、下のバーの5つのどれも選ばれていない見た目になります。上のバーは左が「≡」（引き出し）で、Menuへ戻る印もありません。
  - `screens/phone--keys.png`、`screens/phone--settings-terminal.png`
- **原因**: `const current = section === name;`（`web/src/shell/MobileNavigation.tsx:27`）で、今の区画と5つの名前を直接比べています。Menuの下のページ（Keys、Settingsなど）は、どれとも一致しません。
- **改善案**: 5つに入らない区画のときはMenuを選んだ見た目にします。上のバーの左は「‹ Menu」にし、押すとMenuへ戻るようにします。参考: Apple HIGは、下のバーを出し続けて今どの区画にいるかを示すよう求めています。
- **優先度・規模**: 中・小。

### UX-28 スマホで［新しいセッション］を開くと、キーボードが開く

- **対象**: サイドバーの［新しいセッション］、実行先の切り替え。phone・tablet。
- **今の問題**: どちらのダイアログも、開くと検索欄にフォーカスが入ります（`screens/phone--new-session-dialog.png`の検索欄の枠）。スマホではソフトウェアキーボードが開き、候補の一覧を覆います（キーボードが開くこと自体は撮影のブラウザでは写らない。推定）。
- **原因**: 接続先を選ぶダイアログ`HostPickerDialog`の最初のフォーカスは、指定が無ければ検索欄です。SFTPは狭い画面で［閉じる］にフォーカスするよう渡しています（`web/src/sftp/SFTPHostPicker.tsx:92`、2026-09のmobile-ux-reviewで直した）。［新しいセッション］（`web/src/terminal/WorkspaceTree.tsx:332`）と実行先の切り替え（`web/src/shell/RunTargetSwitcher.tsx:122`）は渡していません。
- **改善案**: 狭い画面では、この2つも［閉じる］に最初のフォーカスを置きます。同じ種類の直し忘れを防ぐため、`HostPickerDialog`の既定を「狭い画面では閉じる」にするのがよいと考えます。方針L98（スマホで勝手にキーボードを開かない）と同じ考え方です。
- **優先度・規模**: 中・小。

### UX-29 Diagnosticsの設定ファイルの一覧で、状態の点が潰れ、長いパスが右で切れる

- **対象**: Diagnostics。phone。
- **今の問題**: 状態を示す緑の点が、細い線に潰れます。切れ目の無い長いパスは折り返さずに右端で切れます（2件目が`osaka.co`で切れている）。
  - `screens/phone--diagnostics.png`
- **原因**: `web/src/diagnostics/DiagnosticsPanel.tsx:195-197`で、点の`span`に`shrink-0`が無く、パスはflexの中の裸の文字で`break-all`がありません。
- **改善案**: 点に`shrink-0`を付け、パスを`min-w-0 break-all`の要素に入れます。UX-08の相対パスと一緒に直します。
- **優先度・規模**: 低・小。

### UX-30 Homeの確認の帯のボタンが文の右にある

- **対象**: Homeの確認の帯と同期の行。全部。
- **今の問題**: ［設定を確認］［同期画面を開く］が文の右に並びます（`screens/tablet--home.png`）。方針L10は「知らせに付ける操作ボタンは知らせの枠の中、文の下」です。
- **原因**: 共通の知らせの部品（`Notice`、`web/src/ui/surface.tsx:142`でボタンを文の下に置く）を使わず、帯を手で組んでいます（`web/src/overview/OverviewPanel.tsx:142-153`）。10-28 Dの「知らせのボタンが文の横」のうち、Noticeの部品は直り、この帯が残りました。
- **改善案**: Homeの帯を`Notice`に載せ替えます。UX-26と一緒に直します。
- **優先度・規模**: 低・小。

### UX-31 復元の知らせに履歴IDが出る

- **対象**: Historyから復元したときの知らせ。全部。
- **今の問題**: 「{path}を復元しました（履歴ID：{id}）。」（`web/src/i18n/messages/ja.ts:1914`）。HistoryのページにIDを出していないので、利用者がこのIDを使う場面がありません（10-28 Dの再掲。当時の3か所のうち2か所は直っている）。
- **改善案**: IDを外します。
- **優先度・規模**: 低・小。

### UX-34 動画・音声のプレビューで、ファイルが無いときも「このブラウザでは再生できない形式です」と出る

- **対象**: ターミナルのプレビュー、SFTPの詳細のプレビュー（動画・音声）。全部。
- **今の問題**: UX-25の崩れで混ざったパス（`…/Pictures/IMG_2041.jpgcording.mp4`、存在しない）を押すと、プレビューは「このブラウザでは再生できない形式です。」と［ダウンロード］を出しました。ファイルが無いのに形式の問題に見え、［ダウンロード］も押せる見た目です。
  - `screens/2in1--terminal-preview-image.png`
- **原因**: 動画・音声は、先に再生用のチケットを発行してから`<video>`／`<audio>`に読ませます（`web/src/sftp/useFilePreview.ts:42-58`）。チケットの発行はファイルがあるかを確かめずに通り（画面が再生の表示に進んだことから。sshcエンジンの側のコードは読んでいない）、その後の再生の失敗は理由を問わず`sftp.previewMediaUnsupported`（`web/src/sftp/FilePreviewMedia.tsx:24-27`）になります。存在しないファイルで読み込みが404になったかは記録していません（推定）。
- **改善案**: チケットを発行するときにファイルがあるか・読めるかを確かめ、無ければ画像やテキストと同じく「プレビューできません」の理由（見つからない）を出します。再生の失敗は、読み込みの失敗と形式の失敗を分けます。
- **優先度・規模**: 低・小。

## 7. 確認できなかったこと・次の段階

- iPadのSafari、AndroidのAPK（WebView）、実機の2in1では確かめていません。UX-01・UX-15・UX-28は実機で見るのがよいです。
- UX-25の原因は推定です。直す前に、sshcエンジンが送る流し直しのバイト列と、そのときの大きさの履歴を記録して確かめます。
- SFTPの詳細でJPEGの［ファイルを編集］を押したときの動き（UX-07）は試していません。
- SSH Config・Diagnosticsの警告が内部の記号と英語のまま出る件（10-28 D）は、今回の架空の設定では警告が出なかったため確かめていません。
- `history`・`vpn`・`remote-keys`・`license`・`about`・`not-found`・Vaultの3ページ・Settingsの残りのページ・ロック解除の画面は、横に並べた画像で見て、所見はありませんでした（Historyは同じ種類の変更が続けて並ぶが、撮影の準備で一度に書いたためで、ふつうの使い方での見え方は確かめていない）。
- 改善案のモックは作っていません。進める番号を選んでもらったら、方針どおり実画面に差し込んだ案を複数並べた画像を作ります。

## 付録A 10-28の点検（担当D）の所見の今の状態

| 10-28の所見 | 今のmain | 根拠 |
|---|---|---|
| Keyboard shortcutsが昔の形 | 直った | `screens/desktop--settings-shortcuts.png` |
| スマホで知らせが下のバーに重なる | 直った | `web/src/ui/ToastProvider.tsx:33`が`--mobile-navigation-inset`を見る |
| Androidのネイティブ画面がライトで白地に白い文字 | 直った | `android/app/src/main/res/values/themes.xml`がLight、`values-night`が暗い方 |
| 知らせのボタンが文の横 | 部品は直った。Homeの帯が残る | UX-30 |
| 知らせに履歴ID | 1か所残る | UX-31 |
| ロック画面・作成画面のサインアウトの2文 | 残る | UX-24 |
| SSH Config・Diagnosticsの警告が内部の記号と英語 | 未確認 | 7章 |

## 付録B 撮影の手順

```sh
cd ~/projects/sshc-worktrees/ux-audit
# Go 1.26 と Node.js 22 を用意する（README.md の「開発」）
npm ci --prefix web && make build
docker build -t sshc-ux-audit-sshd:2026-10-30 scripts/ux-audit/sshd
# HOME とバイナリを置く場所。画像に写るので、ユーザー名を含まない場所にする
UX_AUDIT_SCRATCH=/large/vm/sshc-ux-audit node scripts/ux-audit/capture-screens.mjs artifacts/ux-audit/2026-10-30/screens entry,routes,states,interact,reflow,touch2in1,connections,lock
sh scripts/ux-audit/montage.sh artifacts/ux-audit/2026-10-30/screens artifacts/ux-audit/2026-10-30/sheets phone tablet 2in1 desktop
sh scripts/ux-audit/montage.sh artifacts/ux-audit/2026-10-30/screens artifacts/ux-audit/2026-10-30/sheets-extra phone-landscape tablet-landscape 2in1-tablet laptop
# イシューの登録（まとめを作り、各項目を作り、ほかの項目への参照をリンクにして書き直す）
sh scripts/ux-audit/post-issues.sh <本文の出力先>
```

撮り直しは1回の実行で全部の段を流しました。Homeの確認の件数のもとは`screens/setup.json`の`attention`にあります。
