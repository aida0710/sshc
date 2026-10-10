# ブラウザデモ

公開URLを開き、構成を確認して［起動する］を選ぶと、v86上でLinuxを3台起動する。
sshcエンジンとCLI用に192 MiB、SSH/SFTP接続先のdemo-a・demo-b用に各128 MiBを割り当てる。
VM以外にもブラウザ・エミュレータがメモリを使う。
確認画面の表の下に、初回にダウンロードする合計の容量を表示する。GitHubのデモ一式を取得する場合はその容量（v0.44.1で約41 MB）、配信済みの版を起動する場合はVMの起動ファイルとWeb UIの合計を示す。
起動中はファイルの読み込み割合、3台それぞれの状態、経過時間を表示する。
GitHubの最新リリースへ切り替える時は、デモ一式のダウンロードも同じ起動画面に1行として表示する。
読み込み・Linux起動・エンジン・Web UIの準備が実際に完了したイベントで表示を進める。
画面下部に実際に動くsshcのバージョンと、配信物の生成日時（JST）を表示する。
画面は日本語と英語に対応する。保存した選択、ブラウザの言語の順に決め、どちらでもなければ英語にする。
起動前の画面右上のボタンで切り替えられ、選んだ言語は埋め込みのWeb UIにも渡す。CLIのsshcのメッセージは製品のまま。

VM間のEthernet通信は同じページ内で転送する。HTTPとWebSocketはデモ専用のシリアル通信を経由し、
ゲスト内の127.0.0.1で動く通常のsshcエンジンへ届く。外部のSSHサーバやWebSocketリレーは不要。
Web UIは製品と同じコードを使い、配信したコピーにだけ通信ブリッジを追加する。
UIのファイルは`ui.tar.gz`にまとめ、起動を選んだ後にブラウザで展開してCacheStorageに保存する。
Service Workerが元のURLで返すため、動的import・フォント・エディタのWorkerも製品と同じ構成で動く。
配信用のデモ一式とUIをキャッシュする。VMの操作内容や認証トークンは保存しない。
UIキャッシュがある時は画面下部に削除ボタンを表示する。
選ぶと表示中の版のUIとデモ一式のキャッシュを削除し、公開版の入口に戻る。VMは起動前の確認からやり直す。

接続鍵はこの公開デモ専用の使い捨て鍵。VMイメージに含まれ、秘密ではない。
操作したファイルや設定はVMのメモリ内にあり、リセットまたはページを閉じると消える。
外部ネットワーク・VPN・シリアル機器への接続は用意しない。Webのファイル操作は2 MiBまで。
デスクトップのChromeで起動・Webターミナル・CLI・SFTPの日本語編集を検証する。
モバイルの性能や全ブラウザでの動作は未確認。

## ビルド

Go、Node.js 22以降、Docker、curl、cpio、gzip、GNU tarが必要。
Alpine Linuxの32bitイメージからrootfsを作り、Goのlinux/386ビルドを追加する。
コンテナはイメージ作成にだけ使い、公開デモの実行時は不要。

```sh
VERSION=v0.44.0 bash demo/scripts/build-images.sh
npm ci --prefix web
npm run build --prefix web -- --base=./ --outDir ../demo/images/ui
npm ci --prefix demo
npm run build --prefix demo
npm run serve --prefix demo
```

`http://127.0.0.1:4178/index.html`で開く。生成物は`demo/dist/`。
`SSHC_DEMO_IMAGES_URL`を設定してビルドすると、VMの起動用ファイルだけ別の配信先を指定できる。
配信先が別オリジンの場合は、その配信先でCORSを許可する。全ファイルを同じR2ドメインから配る場合は不要。

```sh
npm test --prefix demo
npm run test:browser --prefix demo
```

ブラウザ検証にはPlaywrightのChromiumが必要。既存のChromiumは`SSHC_DEMO_CHROMIUM`で指定できる。
公開先は`SSHC_DEMO_TEST_URL`、画像の保存先は`SSHC_DEMO_ARTIFACTS`で指定する。
検証は実際のVMで、起動前の取得抑止、両方へのCLI接続、Webターミナル、SFTP編集、リセットを確かめる。
UIアーカイブの取得が1回だけであること、UIがブラウザ内から読み込まれること、展開済みUIの再利用も確かめる。

## Gitで管理する範囲

`demo/src/`は起動画面・進捗・VM通信・UIアーカイブの読み込み、`demo/guest/`はLinuxの構成、
`demo/guestbridge/`はゲスト用ブリッジ、
`demo/scripts/`はイメージ作成・配信物の生成・検証・デプロイを管理する。
`demo/tests/`、`package.json`、`package-lock.json`、このREADMEもGitに含める。
生成物の`demo/images/`、`demo/dist/`、`demo/node_modules/`、検証画像・配信manifestはGitに含めない。

## 配信

GitHub Pages・Vercel・R2など、静的ファイルを配信できる場所で動く。
`.wasm`は`application/wasm`、JavaScriptとCSSは対応するContent-Typeで配信する。
`.cpio.gz`は圧縮済みのディスクファイルなので`Content-Encoding: gzip`を設定しない。
VM実行時に書き込み用APIキーは使わない。
UIアーカイブの展開にはHTTPS（ローカル検証ではlocalhost）とService Worker・DecompressionStreamが必要。

R2はディレクトリのindex.htmlを自動表示しないため、入口は`/index.html`を指定する。
起動画面・UIなどをリリースごとのディレクトリに置き、入口のindex.htmlだけ最後に更新する。
標準構成のVMイメージは内容のSHA-256で決まる共通URLへ配信し、UIだけの更新では再利用する。
配信物のSHA-256一覧を保存し、キー無効化前に匿名の公開URLでブラウザ検証する。

ビルド手順でVMと製品UIを生成した後、次のコマンドで配信物を作り直してR2に公開する。
デプロイにはPythonの`boto3`が必要。

```sh
SSHC_R2_CREDENTIALS_FILE=/安全な場所/r2-credentials.json npm run deploy --prefix demo
```

製品のコードやVMの構成を変えた場合は`-- --rebuild`を付けると、VMイメージと製品UIも再生成する。
初回は先に`npm ci --prefix web`と`npm ci --prefix demo`を実行する。

認証ファイルはGitの外に置き、権限を600にする。形式は次のとおり。

```json
{
  "bucket": "sshc-demo-container",
  "endpoint": "https://<account-id>.r2.cloudflarestorage.com",
  "accessKeyID": "<access-key-id>",
  "secretAccessKey": "<secret-access-key>"
}
```

リリース名はJSTの実行日時から生成する。`-- --release <名前> --manifest <保存先>`で指定もできる。
既存のリリース名は上書きできない。全ファイルのサイズ・SHA-256メタデータを検査してから入口を切り替える。
manifestの既定保存先は`demo/artifacts/r2-manifest-<名前>.json`。

## GitHubのlatestに追従する

入口と予備のデモはR2へ一度配信し、GitHubのReleaseにデモ一式を追加する。
Release workflowの`browser-demo` jobがタグの版を埋め込んだVMとUIを作り、実ブラウザで
SSH・SFTPを検証してから`sshc-demo-<tag>.tar.gz`と`sshc-demo-<tag>.json`を公開する。
この2ファイルもchecksums.txtとattestationの対象になる。

ブラウザは入口を開くたびに、読取専用Workerの`latest.json`を取得する。
WorkerはGitHubのlatest redirectを読み、デモのmanifestを返す。最新の確認は最大5分キャッシュする。
［起動する］を押すと、その版のtar.gzをWorker経由で取得してSHA-256を検査し、展開して保存する。
ブラウザ内のService Workerが`/github-releases/<tag>/`のファイルを返し、同じ版のコードとVMを起動する。
展開済みなら再ダウンロードせず、最新取得に失敗した場合はR2の配信済みデモを使う。
VMイメージを含むReleaseの束では、`SSHC_DEMO_IMAGES_URL`は設定しない。

Workerは指定リポジトリのデモだけをGETで返す。任意URLの中継、VM実行、書き込み、定期実行は行わない。
GitHub/R2の書き込みキーやKVは不要。ブラウザからの直接取得はGitHub ReleaseのCORS制約で失敗するため、
この読取用Workerを置く。

```sh
# 権限600のGit外JSON: {"accountID":"...","apiToken":"..."}
# トークンの権限は対象アカウントのWorkers Scripts: 編集。
SSHC_CF_CREDENTIALS_FILE=/path/to/cloudflare.json python3 demo/scripts/deploy-release-proxy.py
# 出力されたURLが既定値と異なる場合は、ビルド時に指定する。
SSHC_DEMO_RELEASE_PROXY_URL=https://sshc-demo-releases.aida0710.workers.dev/ npm run build --prefix demo
npm run package:release --prefix demo -- ../dist
```

Worker公開後、通常のR2配信スクリプトで入口を更新する。
Worker登録に必要な`demo-release-worker.js`と`ui-cache-addresses.js`もルートへ保存する。
以後は通常のGitHub Releaseだけでデモが更新され、リリースごとのR2アップロードは要らない。
配信形式やルートService Workerの仕様を変える場合は、Worker・R2の入口も更新する。

Workers Freeの上限はアカウント全体で1日10万リクエスト、1回CPU 10ms。
通常は1回の閲覧でlatest確認1回と初回のアーカイブ取得1回がWorkerを呼ぶ。
展開とVMの実行は利用者のブラウザで行う。現在の上限は
[Workers limits](https://developers.cloudflare.com/workers/platform/limits/)で確認する。
