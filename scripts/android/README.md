# Android runtime test

`run-vault-lifecycle-test.sh`は、Android API 36のdebug APKを実際に起動し、WebViewをDevTools protocol経由で操作して次を検証します。

1. アプリdataを消した状態でvaultを作成する
2. アプリを強制終了する
3. 再起動後に同じマスターパスワードでvaultを解除する
4. 各操作後の画面を`artifacts/android-vault-lifecycle/`へ保存する

KVMを利用できるAPI 36 Emulatorを起動し、debug APKを作成してからrepository rootで実行します。

```sh
export ANDROID_SDK_ROOT=/path/to/android-sdk
export SSHC_ANDROID_NODE=/path/to/node
scripts/android/run-vault-lifecycle-test.sh android/app/build/outputs/apk/debug/app-debug.apk
```

テスト用マスターパスワードは固定fixtureです。実データや利用者の認証情報は読みません。接続中の端末がAPI 36 Emulatorでなければ、誤って実機dataを消さないよう操作前に停止します。

## 実際のS3互換ストレージから受信する

`sync-runtime-test.mjs`は、Android WebView上の同期設定と受信専用同期を検証します。認証情報はコマンドライン引数やファイルではなく、次の環境変数だけから読みます。

```sh
export SSHC_SYNC_TEST_ENDPOINT=https://object-storage.example
export SSHC_SYNC_TEST_BUCKET=example-bucket
export SSHC_SYNC_TEST_ACCESS_KEY_ID=...
export SSHC_SYNC_TEST_SECRET_ACCESS_KEY=...
export SSHC_SYNC_TEST_KEY=...
export SSHC_WEBVIEW_DEBUG_ENDPOINT=http://127.0.0.1:9222
```

新規インストールの設定フォームへ入力した後、Androidのホーム画面へ移動してからアプリへ戻し、値が失われていないことを検証します。

```sh
node scripts/android/sync-runtime-test.mjs prepare
# Androidのホーム画面へ移動してからsshcへ戻す
node scripts/android/sync-runtime-test.mjs verify-and-pull
```

設定済みの受信専用端末は、次のモードで再検証できます。受信の前後でlive objectを比較し、バケットへ書き込んでいないことも確認します。`SSHC_SYNC_TEST_LOCALE=ja`を指定すると日本語表示で実行できます。

```sh
SSHC_SYNC_TEST_LOCALE=ja node scripts/android/sync-runtime-test.mjs run-configured
```

実運用の保存先を使う場合も、必ず受信専用のテスト端末で行ってください。スクリプトは認証情報や同期キーを出力しません。
