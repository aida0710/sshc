# スマホ操作レビュー

2026-09-12 JST。対象は `agent/mobile-ux-dev` の開発版 `0.33.2-mobile.1-dev`。正式版 v0.33.2 の後に行った変更で、まだ正式リリースとして公開していません。

目的は、指での操作に応答を返し、一覧や入力対象に画面を使えるようにすることです。スマホ幅のブラウザーとAndroid WebViewで共通のUIを使います。以下の「確認項目」は観点の一覧です。実行結果と未確認の範囲は後段で区別しています。

## 画面ごとのレビュー

| 画面・操作 | 問題 | 今回の変更 | 確認項目 |
| --- | --- | --- | --- |
| 全画面の移動 | 主要画面の切り替えに毎回drawerを開く必要がある | Home／Connections／SFTP／Terminal／Menuを下部に常設 | 選択表示、未保存の編集の確認、戻る履歴、各ページへの到達 |
| キーボード表示 | 固定の画面高さや上下の操作欄が入力領域を圧迫する | visualViewportを共有し、IME検出中はヘッダーと下部ナビを隠す | フォーカス移動、IME開閉、回転、入力欄と確定ボタンが画面内に残ること |
| タッチ対象と応答 | 小さなボタン、押下を受け付けたか分かりにくい操作 | 原則44pxの対象、通常入力欄16px、押下状態の表示 | 320px幅、指で隣の操作を誤って押さないこと、長い日本語ラベル |
| ダイアログ・シート | 短い画面で内容が画面外へ出る。子メニューが親と競合する | 表示領域内の高さ制限と内部スクロール、子レイヤーを維持 | 最前面だけをAndroid Back／Escapeで閉じる、Tabの範囲、操作元へのフォーカス復帰 |
| Home | 行メニューがスクロール領域や画面端で切れる | メニューをportalに置き、可視領域内へ位置調整 | 最上段・最下段のメニュー、スクロール／回転後の位置、接続開始 |
| Connections | 横向きのスマホで2ペインになり、一覧と編集の幅が足りない | タッチ端末の横向きも単一ペインで一覧と編集を切り替える | 一覧へ戻る、編集内容と選択の保持、desktopの2ペイン維持 |
| SFTP一覧 | 常設ボタンと転送管理が一覧を押し縮める | 通常ツールバーを1行、二次操作はシート、検索は必要時に表示 | 一覧の高さ、フォルダ作成・upload・場所・履歴・並べ替えへの到達 |
| SFTPのタップ | 名前タップで選択、ダブルタップで開くため操作が分かれにくい | 名前の1タップで開く／preview。checkbox・長押しで選択し、選択中はタップで追加・解除 | 誤ったフォルダ移動がないこと、複数選択、長押し後のclick抑止、desktop操作維持 |
| SFTPの通信待ち | 移動中も古い一覧が操作でき、受付が見えない | 読込先と進行表示、待機中の一覧をinertにする | 遅延・失敗・古い応答、待機中の二重操作防止、失敗時のpath維持 |
| SFTP転送管理 | 展開すると一覧が23pxまたは0pxまで縮む | 1行dockから別sheetへ展開。設定はsheet内で折り畳む | 開閉前後で一覧の高さが変わらないこと、全ジョブ操作、子メニュー、短い横向き画面 |
| SFTP preview | 属性一覧がpreview領域を押し縮める | Propertiesを折り畳み、previewを伸ばす | 長いパス・本文・画像、属性展開、編集／downloadへの遷移 |
| SFTP接続先選択 | 選ぶだけでも検索欄のfocusでIMEが開く | スマホではCloseへ初期focus | IMEを開かず候補選択、検索をタップした場合の入力、キャンセル |
| Terminalのスクロール | 指を離すと突然止まり、タッチの流れが途切れる | 速度に応じた減速、再タッチ・選択・非表示時に停止 | 上下方向、途中で停止、選択・画面遷移中の停止、reduced-motion |
| Terminalの補助キー | 横向きで消える、操作列が高さを使う、IMEのfocusが動く | Ctrl／Esc／Tab／矢印を1行、Altと記号は展開。押下でfocusを維持 | 横向き、Ctrl＋文字、Alt、Tab、矢印、IMEが閉じたり点滅したりしないこと |
| Terminalの選択 | 選択解除や再タップでblur／focusが繰り返される | 不要なfocus変更を避け、選択中のスクロールと慣性を制御 | 長押し選択、範囲調整、copy、解除、通常入力への復帰 |
| 資格情報 | 行メニューがカードや画面端で切れる | Homeと同じ位置調整付きportalを使用 | 編集／削除確認への到達、最下段、外側タップとBack |
| Config Explorer | 常時表示の階層一覧が編集領域を圧迫し、読込と空状態が区別しにくい | 狭い画面では階層を折り畳み、ファイルを開くと閉じる。読込表示を追加 | 階層の開閉、再選択、遅延・失敗、編集欄への到達 |
| Snippets | 全件一覧・固定列・長い名前が編集画面を押し広げる | 狭い画面は選択欄へ置換、変数と接続先を1列化。実行中は実行ボタンを無効化 | 選択と新規作成、長い変数／接続先、previewと確認、二重実行防止 |
| Settings／License／Keys／履歴など | 共通の入力・ボタン・modalの制約を受ける | 共通のタッチ対象とviewport対応を適用 | 長い画面のスクロール、検索、フォーム・確認操作。個別の挙動は既存契約を維持 |

SFTPの旧状態は390×640pxで転送管理を展開すると一覧が23px、390×480pxでは0pxになっていました。新しいsheetは一覧のレイアウト外に表示するため、開閉で一覧を押し縮めません。ブラウザーでは640px・480pxの両方で開閉前後の一覧の高さと位置が一致しました。API 36エミュレーターの412×842pxでは579.476pxの高さを維持しました。

## 検証記録

### コンポーネントと静的検査

- SFTP関連4ファイル／65テストが成功。転送sheet、子メニュー、単一タップ、選択、遅延読込、古い応答、desktopの操作を含みます。
- 最後に追加した「接続先pickerでIME用入力欄を自動focusしない」確認を含むHostPicker 3テストも成功しました。SFTP関連の現在の合計は66テストです。
- SFTPのESLint、全Web TypeScript、変更差分の空白検査が成功しました。
- Android runtime runnerの対象選択をfake SDK／adbで検査する6テストも成功しました。Dev／旧debugの識別、release／別package／異なるActivityの拒否、実機へのデータ削除防止を含みます。
- 最終Web全134ファイル／1,298テスト、ESLint、TypeScript、`make verify-generated`、日英文書ビルドが成功しました。Goのbuildcontract・acceptance・mobile、Gradleの`testDebugUnitTest`／`assembleDebug`も成功しています。

### ブラウザー／Android

- Chromiumの新規mobile 4件、既存narrow 19件、関連desktop 71件が成功。最後のAndroid向けTerminal修正後にはmobile 4件・narrow Terminal 5件・desktop描画5件を再検証しました。SFTPの遅延応答と転送データはこのブラウザー試験では合成しています。
- API 36／x86_64の専用AndroidエミュレーターへDev APKをインストール。パスワードレスVault作成と再インストール後の直接起動、フォームのIME開閉・入力欄切替・入力欄の可視範囲、Android Backを確認しました。
- 同エミュレーターでローカルPTYの入力、縦横の補助キー、IMEを開いたままの回転と復帰、横画面での再タップ・Backを確認しました。412×530pxと866×127pxのIME表示領域でもナビを隠したまま入力focusを維持します。
- localhost限定の専用OpenSSHコンテナへAndroidから接続し、SFTPのフォルダ移動、テキストpreviewの内容一致、転送シートの開閉とBackを確認しました。公開鍵はそのコンテナから取得して照合し、資格情報・ファイルはすべてテスト専用です。
- Android検証で、WebGLにDOM行がないと選択領域が古い寸法で残る問題、IMEによる親要素のスクロール、resize後の入力textareaが画面外へ残る問題を発見。renderer非依存の計測、`overflow-clip`、resize後の入力座標補正を追加しました。
- 初期AVDの`swiftshader_indirect`ではglyphが斜めに欠けました。アプリを介さない単純な四角形でも`TRIANGLE_STRIP`の半分だけが欠け、`TRIANGLES`とdesktop Chromiumでは正常だったため、エミュレーターGPU経路の問題と切り分けました。既存のcontext-loss fallbackでDOM描画へ戻ると正常表示となり、Android経由の文字入力と独立したshell出力行も確認できました。
- 専用AVDを`-gpu swangle`で起動すると、四角形の描画テストは全12条件で正常となりました。最終APKの既定WebGL描画でも、Android経由の文字入力と独立したshell出力、IMEを開いたままの縦横回転・再タップ・Backを再確認しています。CDPの画像取得ではcanvasが黒く写るため、確認画像には`adb exec-out screencap -p`を使用しました。製品側の描画設定は変更していません。
- 実際のスマートフォン本体とiOS Safariは未確認です。エミュレーターでの結果と実機の描画・操作感を区別します。

## 開発版APK

| 項目 | 開発版 | 正式版 |
| --- | --- | --- |
| アプリ名 | sshc Dev | sshc |
| applicationId | `com.github.aida0710.sshc.dev` | `com.github.aida0710.sshc` |
| namespace／Activity class | `com.github.aida0710.sshc`／`com.github.aida0710.sshc.MainActivity` | 同じ |
| 今回のversionName | `0.33.2-mobile.1-dev` | 公開済みは`0.33.2` |
| データ | Dev専用のVault・設定・WebView storage | 正式版専用のデータ |

[確認用APK](https://drive.google.com/file/d/1gQaGug3-PdXqW4VbUwPBof0XpwZNBqrG/view)と[画像・SHA256SUMS・ビルド情報](https://drive.google.com/drive/folders/1tg3JwoUeXA8Tr99VxR0SjXWBeGNZYZVd)を共有しています。APKのソースは`e39fdc91ce879e44d93bd3da87825101f0523b44`、engine表示は`v0.33.2-mobile.1-dev+e39fdc91`です。

2つのアプリは並べてインストールできます。Devを入れても正式版のデータを上書きせず、正式版からの自動移行も行いません。開発版の署名はGradleのdebug署名です。

リポジトリ所定のGo・Node・Java・Android SDK／NDKを用意し、repository rootでUIを先に生成します。`android-bind`だけではWeb UIを再生成しません。

```sh
make build
make android-bind ANDROID_VERSION=0.33.2-mobile.1-dev
cd android
./gradlew testDebugUnitTest assembleDebug -PsshcVersionName=0.33.2-mobile.1
```

APKは `android/app/build/outputs/apk/debug/app-debug.apk` に出力されます。Gradleが`-dev`を付けるため、`sshcVersionName`へ重ねて付けません。別のSDK／NDK配置を使う場合は、既存のAndroidビルド手順に従って`ANDROID_NDK_HOME`等を指定します。

リポジトリのrootへ戻り、テスト端末へインストールして起動します。

```sh
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n com.github.aida0710.sshc.dev/com.github.aida0710.sshc.MainActivity
```

## 確認が残る範囲

- 実端末でのGboard／各メーカーIME、予測変換、日本語入力、外付けキーボードとの切り替え。IME検出はfocusと高さ変化からの推定です。
- Androidのシステムバー、ジェスチャー／3ボタンナビ、分割画面、フォルダブル端末での可視領域と戻る順序。
- 実端末でのフレームの滑らかさと長いscrollback、長いファイル一覧、遅いSSH接続での体感。慣性追加だけで性能改善を保証しません。
- 実Androidのファイルpicker、保存先権限、長い転送のバックグラウンド継続と通知。転送engineと既存のSAF契約は今回変更していません。
- iOSネイティブアプリは配布対象外です。iOS Safariを含むブラウザーごとのIME・タッチの実機確認は別途必要です。
