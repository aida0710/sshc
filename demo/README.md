# ブラウザデモ

公開URLを開き、構成を確認して［起動する］を選ぶと、v86上でLinuxを3台起動する。
sshcエンジンとCLI用に192 MiB、SSH/SFTP接続先のdemo-a・demo-b用に各128 MiBを割り当てる。
初回ダウンロードは約40 MB。VM以外にもブラウザ・エミュレータがメモリを使う。

VM間のEthernet通信は同じページ内で転送する。HTTPとWebSocketはデモ専用のシリアル通信を経由し、
ゲスト内の127.0.0.1で動く通常のsshcエンジンへ届く。外部のSSHサーバやWebSocketリレーは不要。
Web UIは製品と同じコードを使い、配信したコピーにだけ通信ブリッジを追加する。

接続鍵はこの公開デモ専用の使い捨て鍵。VMイメージに含まれ、秘密ではない。
操作したファイルや設定はVMのメモリ内にあり、リセットまたはページを閉じると消える。
外部ネットワーク・VPN・シリアル機器への接続は用意しない。Webのファイル操作は2 MiBまで。
デスクトップのChromeで起動・Webターミナル・CLI・SFTPの日本語編集を検証する。
モバイルの性能や全ブラウザでの動作は未確認。

## ビルド

Go、Node.js 22以降、Docker、curl、cpio、gzipが必要。
Alpine Linuxの32bitイメージからrootfsを作り、Goのlinux/386ビルドを追加する。
コンテナはイメージ作成にだけ使い、公開デモの実行時は不要。

```sh
bash demo/scripts/build-images.sh
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

## 配信

GitHub Pages・Vercel・R2など、静的ファイルを配信できる場所で動く。
`.wasm`は`application/wasm`、JavaScriptとCSSは対応するContent-Typeで配信する。
`.cpio.gz`は圧縮済みのディスクファイルなので`Content-Encoding: gzip`を設定しない。
VM実行時に書き込み用APIキーは使わない。

R2はディレクトリのindex.htmlを自動表示しないため、入口は`/index.html`を指定する。
リリースごとのディレクトリに全配信物を置き、入口のindex.htmlだけ最後に更新する。
配信物のSHA-256一覧を保存し、キー無効化前に匿名の公開URLでブラウザ検証する。
