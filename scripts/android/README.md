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
