# スマホ版（Android のアプリ）の点検の道具（2026-11-03）

エミュレータに入れた開発版のアプリを、指の操作（adb）と WebView の CDP で動かして撮る。点検の所見は #657。

- `cdp.sh`: アプリの WebView の DevTools を 127.0.0.1:9223 に転送する。
- `run.mjs <step.mjs> [nosnap]`: CDP でつなぎ、手順のモジュールの default(page) を流す。台本のある場所の node_modules を使うので、パスを自分の作業ツリーに合わせる。
- `device.mjs`: ページの座標で指の操作を送る（calibrate は wm size から位置を測る）、screencap で撮る、キーボードが開いているかを見る。
- `nav.mjs`・`claude-keys.mjs`: 画面の移動、キーバーのキー、入力欄から送る・空で送る（Enter）。
- `gen-home.py`・`push-home.sh`: アプリの HOME（files/）へ整ったデータか荒れたデータの ~/.ssh を run-as で入れる。
- `m10-routes.mjs`・`measure.mjs`: 全画面を撮り、押す場所・切れた文字・はみ出しを数える（ROUTE_PREFIX、ROUTE_ONLY）。
- `mock-anthropic.py`: Claude Code の画面を動かすための作りものの Messages API。モデルには送らない。依頼の文に「一覧」があると Bash の道具の呼び出し（許可の確認）を返し、道具の結果のあとに長い答えを返す。/tmp/mock-delay に秒数を書くと呼び出しを返す前に待つ。コンテナの中で `python3 mock-anthropic.py`（127.0.0.1:8099）、Claude Code は ANTHROPIC_BASE_URL=http://127.0.0.1:8099 とダミーの ANTHROPIC_API_KEY、~/.claude.json に hasCompletedOnboarding と customApiKeyResponses.approved（鍵の末尾20文字）を書いて起動する。
