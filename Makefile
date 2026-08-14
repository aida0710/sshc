.PHONY: generate test build desktop icons desktop-run desktop-dist release-binaries fuzz e2e verify-generated integration integration-up integration-down integration-sshd-relax install install-binary uninstall uninstall-binary update

# FUZZTIME は target ごとの時間である。`make fuzz` は単発の実行ではなくキャンペーン
# なので、既定値は通常の検証パスの一部として回せる程度に短くしてある。腰を据えて
# 走らせるときは引き上げること: `make fuzz FUZZTIME=10m`。
FUZZTIME ?= 30s

# リポジトリ内のすべての fuzz target を package:Target の形で列挙する。target を
# 追加してここに加え忘れると TestMakefileFuzzTargetsCoverEveryFuzzFunction が失敗する。
FUZZ_TARGETS = \
	internal/config:FuzzParseRendersOriginalBytes \
	internal/config:FuzzExpandIncludePattern \
	internal/effective:FuzzParseValues \
	internal/effective:FuzzResolve \
	internal/knownhosts:FuzzParseKnownHostsRoundTrip \
	internal/acceptance:FuzzAPIRequestBodies \
	internal/remotesync:FuzzReadSnapshot


generate:
	go generate ./internal/api
	npm run generate:api --prefix web

test:
	go test ./...
	go test -race ./...
	npm test --prefix web
	npm run typecheck --prefix web
	@# 外殻の JS も検査する。**relink は利用者の ~/.local/bin に触る**ので、
	@# 「普通のファイルには触らない」という規則は落とせない。node 標準の
	@# テストランナーなので、依存は増えない。
	npm test --prefix desktop

fuzz:
	@set -e; for target in $(FUZZ_TARGETS); do \
		package="$${target%%:*}"; \
		name="$${target##*:}"; \
		echo "==> fuzz $$package $$name for $(FUZZTIME)"; \
		go test "./$$package" -run '^$$' -fuzz "^$$name$$" -fuzztime "$(FUZZTIME)"; \
	done

e2e: build
	npm run e2e --prefix web

# verify-generated は API のモデルを再生成し、コミット済みのものと異なれば失敗する。
# api/openapi.yaml が、Go のモデルと TypeScript の型の両方にとって依然として唯一の
# 源であることの証明である。
verify-generated: generate
	git diff --exit-code -- internal/api/models.gen.go web/src/api/schema.d.ts

# VERSION は、ビルドされたバイナリが報告する値であり、更新確認が比較する対象でも
# ある。タグのないビルドは "dev" と名乗る。それにはリリースがあることは伝えるが、
# どれだけ遅れているかは決して伝えない。
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo dev)
export VERSION

build:
	npm run build --prefix web
	mkdir -p bin
	go build -trimpath -ldflags "-X main.version=$${VERSION}" -o bin/sshc ./cmd/sshc

# デスクトップの外殻。
#
# **束ごとに入る sshc は、その束のプラットフォームのものでなければならない。**
# 一つを使い回すと、Linux の AppImage に macOS のバイナリが入る——ビルドは
# 通り、配ってから初めて壊れる種類の間違いである。electron-builder の
# ${os}-${arch} がその選択を行うので、こちらはその名前で置く。
DESKTOP_BUNDLES = mac-arm64:darwin:arm64:1 mac-x64:darwin:amd64:1 \
	linux-x64:linux:amd64:0 linux-arm64:linux:arm64:0

# desktop は、外殻を動かすのに要るものを揃える。開発中はホストの bin/sshc を
# 使うので、束ごとのバイナリはここでは作らない。
desktop:
	npm install --prefix desktop

# icons は SVG からアプリの図を焼き直す。
#
# **正本は desktop/build/icon.svg である。** PNG は焼いたものだが、束を作るのに
# 要るのでコミットしてある——`internal/ui/dist` と同じ扱いである。図を直したら
# これを走らせる。焼くのに使うのは web が持っている Chromium なので、
# 変換のためだけの依存は無い。
icons:
	npm run icons --prefix desktop

# desktop-run は、束を作らずにその場で外殻を開く。開発中の入口である。
#
# **走っているアプリがあるなら、先に終わらせる。** 二度目の `npm start` は
# `requestSingleInstanceLock` に弾かれてすぐ消えるので、焼き直したものは走ら
# ないまま、先に居るアプリが古いエンジンで画面を配り続ける。メニューバーの
# 「終了」で畳めば、次の起動が新しい実体を上げる。
#
# **だから install を通す**——理由は端末の側にある。`sshc` と打った人が走らせる
# のは `~/.local/bin/sshc` であって、この checkout の bin/sshc ではない。外殻は
# 起動のたびに relink を試すが、`desktop/link.js` はそこに実体（symlink では
# ないもの）があるなら触らない——`make install` を一度でも通した機械では、
# 画面だけが新しく、端末は古い版を走らせ続ける。install-binary がそこを
# 入れ替える。
#
# install が失敗しても止まらない。**ここで要るのは外殻が新しいことだけ**で
# あり、それは bin/sshc を焼いた時点で済んでいる（外殻はそれを直接起こす）
# ——端末側の実体を入れ替えるのは、それとは別の、外の世界に効く操作である。
# あれが転んだからといって、アプリを開けない理由にはならない。ビルドの失敗は
# ここに含めない（それは下の build が先に落ちる）。
desktop-run: build desktop
	-@$(MAKE) --no-print-directory install-binary \
		INSTALL_SOURCE="$(CURDIR)/bin/sshc" INSTALL_DIR="$(INSTALL_DIR)"
	npm start --prefix desktop

# desktop-dist は配布物を作る。**1 台の macOS から macOS と Linux の両方を
# 作れる**——それが Tauri ではなく Electron を選んだ理由であり、実際に
# 確かめてある（DMG と AppImage が同じ実行で出る）。
desktop-dist: desktop
	npm run build --prefix web
	@set -eu; for bundle in $(DESKTOP_BUNDLES); do \
		name="$${bundle%%:*}"; rest="$${bundle#*:}"; \
		goos="$${rest%%:*}"; rest="$${rest#*:}"; \
		goarch="$${rest%%:*}"; cgo="$${rest##*:}"; \
		echo "==> desktop/resources/$$name ($$goos/$$goarch)"; \
		mkdir -p "desktop/resources/$$name"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED="$$cgo" \
			go build -trimpath -ldflags "-X main.version=$${VERSION}" \
			-o "desktop/resources/$$name/sshc" ./cmd/sshc; \
	done
	@# **版はひとつである。** 束の中の sshc と、その束自身が別の版を名乗ると、
	@# どちらが本当かを言えるものが無くなる。dev のときは package.json の
	@# 既定のままにする——npm は "dev" を版として受け付けない。
	@if [ "$${VERSION}" != "dev" ]; then \
		npm version --prefix desktop --allow-same-version --no-git-tag-version \
			"$${VERSION#v}" >/dev/null; \
		echo "desktop version -> $${VERSION#v}"; \
	fi
	npm run dist --prefix desktop

# リリースの成果物。UI のバンドルは 1 度だけ作り、Go だけをターゲットごとに
# ビルドする。バンドルは埋め込まれるだけで、どの OS 向けかを知らないからだ。
#
# **darwin は cgo を有効にしたままにする。** 設定エンジンは `%u` と `%i` を
# 展開するために os/user.Current() を読む。CGO_ENABLED=0 の Go は代わりに
# /etc/passwd を読み、macOS の通常のアカウントはそこに載っていない。ビルドは
# 通り、テストも通り、実行時にそのトークンだけが黙って非対応になる——リリース
# でだけ起きる種類の壊れ方である。Linux は /etc/passwd が本物なので 0 でよい。
#
# darwin/amd64 を arm64 のランナーから作れるのは、macOS の SDK が両方の
# アーキテクチャを持っているからである。
RELEASE_TARGETS = darwin/arm64:1 darwin/amd64:1 linux/amd64:0 linux/arm64:0

release-binaries:
	npm run build --prefix web
	mkdir -p dist
	@set -eu; for target in $(RELEASE_TARGETS); do \
		platform="$${target%%:*}"; cgo="$${target##*:}"; \
		goos="$${platform%%/*}"; goarch="$${platform##*/}"; \
		echo "==> $$goos/$$goarch (CGO_ENABLED=$$cgo)"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED="$$cgo" \
			go build -trimpath -ldflags "-X main.version=$${VERSION}" \
			-o "dist/sshc-$$goos-$$goarch" ./cmd/sshc; \
	done

# 統合テストのスイートは、コンテナ内の本物の S3 実装と本物の sshd に対して走る。
# 密閉されたスイートには答えられない二つの問いに答える。本物のオブジェクトストアが
# 条件付き PUT に何をするのか、そして askpass ヘルパーが、暗号化された秘密鍵を
# 保存済みの鍵パスフレーズで実際に開いて認証を通せるのか、である。
#
# どちらのスイートも、環境変数が設定されていなければスキップする。したがって
# `make test` は密閉されたまま、オフラインのままである。
# タグは同じ名前のまま別の内容を指せる。統合テストが昨日と今日で異なる
# サーバーを起動しないよう、マルチアーキテクチャの manifest digest を固定する。
S3_IMAGE   ?= chrislusf/seaweedfs@sha256:43b768cd62b00d132439cda881b93fd1adebf1b315e996e794087743821d771d
SSHD_IMAGE ?= linuxserver/openssh-server@sha256:96b9a4d3b5106746d08d43a6911650d4d21f7d5c7f2ac9660e792bdb5e63157c
S3_PORT    ?= 8333
SSHD_PORT  ?= 2222
S3_KEY     ?= SSHUITESTKEY
S3_SECRET  ?= sshuitestsecret
SSH_USER   ?= tester
SSH_PASS   ?= integration-only-password
SSH_KEY_PASSPHRASE ?= integration-key-passphrase

integration-up:
	@printf '{"identities":[{"name":"sshc","credentials":[{"accessKey":"$(S3_KEY)","secretKey":"$(S3_SECRET)"}],"actions":["Admin","Read","Write","List","Tagging"]}]}' > .integration-s3.json
	docker rm -f sshc-s3 sshc-sshd >/dev/null 2>&1 || true
	docker run -d --name sshc-s3 -p 127.0.0.1:$(S3_PORT):8333 \
		-v "$(PWD)/.integration-s3.json:/etc/seaweedfs/s3.json:ro" $(S3_IMAGE) \
		server -s3 -s3.port=8333 -s3.config=/etc/seaweedfs/s3.json -dir=/data
	docker run -d --name sshc-sshd -p 127.0.0.1:$(SSHD_PORT):2222 \
		-e PASSWORD_ACCESS=true -e USER_NAME=$(SSH_USER) -e USER_PASSWORD=$(SSH_PASS) \
		$(SSHD_IMAGE)
	@rm -f .integration-key/id_integration .integration-key/id_integration.pub
	@mkdir -p .integration-key
	@ssh-keygen -q -t ed25519 -N "$(SSH_KEY_PASSPHRASE)" \
		-f .integration-key/id_integration -C sshc-integration
	@echo "waiting for the containers to answer"
	@for i in $$(seq 1 60); do \
		curl -s -o /dev/null http://127.0.0.1:$(S3_PORT)/ && break; sleep 1; done
	@for i in $$(seq 1 60); do \
		(exec 3<>/dev/tcp/127.0.0.1/$(SSHD_PORT)) 2>/dev/null && break; sleep 1; done
	@$(MAKE) --no-print-directory integration-sshd-relax
	@docker exec -i sshc-sshd sh -c ' \
		umask 077; \
		install -d -m 700 /config/.ssh; \
		touch /config/.ssh/authorized_keys; \
		chmod 600 /config/.ssh/authorized_keys; \
		cat >> /config/.ssh/authorized_keys' < .integration-key/id_integration.pub

# OpenSSH 10 は PerSourcePenalties を既定で有効にし、このイメージは 10.3 を積んで
# いる。ペナルティは送信元アドレスごとに課され、認証せずに切断する接続（すべての
# ssh-keyscan がそれにあたる）と、認証に失敗する接続（ここのテストのうち二つが
# 意図的にそうする）が対象になる。スイート全体が数秒のうちにひとつのアドレスから
# 来るので、ペナルティは閾値を超えて積み上がり、sshd は実行の途中から接続を拒否し
# 始める。このスイートの初回 CI 実行は、まさにそうやって三つ目のテストで失敗した。
#
# 自分のサーバーへ接続する人がこんなことをするわけではないので、このペナルティが
# 測っているのは製品ではなくテストハーネスである。そこでコンテナに限って無効に
# し、その結果は仮定せずに確認する。ディレクティブはイメージ内のすべての
# sshd_config へ書き込む。このイメージが sshd をどれで起動するかはイメージ側の
# 事情であって、決め打ちする価値のあるものではないからだ — 最初の試みは文書化
# されたパスを当て推量し、イメージはそれを移動させていた。
integration-sshd-relax:
	@docker exec sshc-sshd sh -c ' \
		found=$$(find /config /etc /defaults -name sshd_config 2>/dev/null); \
		if [ -z "$$found" ]; then \
			echo "this image has no sshd_config; the penalty cannot be turned off" >&2; \
			exit 1; \
		fi; \
		for configuration in $$found; do \
			grep -q "^PerSourcePenalties" "$$configuration" || \
				printf "\nPerSourcePenalties no\n" >> "$$configuration"; \
		done; \
		echo "PerSourcePenalties no ->" $$found'
	docker restart sshc-sshd
	@for i in $$(seq 1 60); do \
		ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null | grep -q . && break; sleep 1; done
	@# The check is the failure mode itself. Eight scans in a row from one
	@# address is more than the whole suite makes; with the penalty still on,
	@# sshd starts refusing part way through and this says so here, where the
	@# cause is obvious, instead of in the middle of a test.
	@for i in 1 2 3 4 5 6 7 8; do \
		ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null | grep -q . || { \
			echo "sshd refused connection $$i of 8 from one address: the per-source penalty is still on" >&2; \
			exit 1; \
		}; \
	done
	@echo "sshd accepts repeated connections from one address"

# 統合テストのコンテナが使う資格情報は開始時に書き出されるもので、秘密ではなく
# フィクスチャである。このファイルをコミットせず無視するのは、その名前が、誰かが
# 本物の鍵を置く場所になってしまわないようにするためである。
integration-down:
	docker rm -f sshc-s3 sshc-sshd >/dev/null 2>&1 || true
	rm -f .integration-s3.json
	rm -f .integration-key/id_integration .integration-key/id_integration.pub
	rmdir .integration-key 2>/dev/null || true

# 最初の PUT より前にバケットが存在していなければならない。クライアントに意図的に
# CreateBucket がないのは、アプリケーションもバケットを作らないからである。
#
# sshd の側が確かめるのは、**自分で話す SSH が本物の OpenSSH に通じること**で
# ある。単体テストの相手は Go で書かれたサーバー——実装のもう半分——なので、
# 両方が同じ勘違いをしていれば緑になる。ここだけがその輪の外にある。
integration: build
	SSHC_TEST_S3_ENDPOINT=http://127.0.0.1:$(S3_PORT) \
	SSHC_TEST_S3_KEY=$(S3_KEY) \
	SSHC_TEST_S3_SECRET=$(S3_SECRET) \
	SSHC_TEST_S3_BUCKET=sshc-test \
	SSHC_TEST_SSH_ADDR=127.0.0.1:$(SSHD_PORT) \
	SSHC_TEST_SSH_USER=$(SSH_USER) \
	SSHC_TEST_SSH_PASSWORD=$(SSH_PASS) \
	SSHC_TEST_SSH_KEY="$(CURDIR)/.integration-key/id_integration" \
	SSHC_TEST_SSH_KEY_PASSPHRASE="$(SSH_KEY_PASSPHRASE)" \
	go test ./internal/objectstore ./internal/remotesync ./internal/sshintegration -count=1 -v

# バイナリはひとつの安定したパスへ置く。デスクトップの外殻はここへ symlink を
# 張り、CLI と画面が同じ実体を走らせることを保証する。別の場所でビルドし直すと
# 版がずれ、どちらが走っているのか分からなくなる。
#
# ~/.local/bin は sudo もシステムディレクトリの所有権も必要としない。PATH に
# 入っていない場合は、誰も見ない場所へインストールするのではなく、その旨を告げる。
INSTALL_DIR ?= $(HOME)/.local/bin
INSTALL_SOURCE ?= bin/sshc

install: build
	@$(MAKE) --no-print-directory install-binary \
		INSTALL_SOURCE="$(CURDIR)/bin/sshc" INSTALL_DIR="$(INSTALL_DIR)"
	@case ":$$PATH:" in \
		*":$(INSTALL_DIR):"*) ;; \
		*) echo "note: $(INSTALL_DIR) is not on PATH; add it to run 'sshc <alias>' by name" ;; \
	esac

# build済みバイナリを同じdirectory内へstageしてからrenameする。実行中の古いinodeを
# 壊さず、途中でcopyに失敗しても既存のCLIを半分だけ書き換えない。分離targetなのは、
# fake binaryと一時directoryでこの境界を実行テストするためである。
install-binary:
	@set -eu; \
		destination="$(INSTALL_DIR)/sshc"; \
		if [ -d "$$destination" ]; then \
			echo "sshc: install destination is a directory: $$destination" >&2; \
			exit 1; \
		fi; \
		mkdir -p "$(INSTALL_DIR)"; \
		temporary=$$(mktemp "$(INSTALL_DIR)/.sshc.install.XXXXXX"); \
		trap 'rm -f "$$temporary"' 0 1 2 15; \
		install -m 0755 "$(INSTALL_SOURCE)" "$$temporary"; \
		mv -f "$$temporary" "$$destination"; \
		trap - 0 1 2 15; \
		echo "installed $$destination"

# ソースからのチェックアウトにおける更新とは、取得し直してインストールし直すこと
# である。アプリケーション自身の更新ボタンとは別物だ。あちらは、ビルドするソースを
# 持たない人のためにリリース済みのバイナリを置き換えるものである。
#
# ローカルのブランチが進んでいる場合、--ff-only はマージコミットを勝手に作らずに
# 拒否する。「make update」が、誰も頼んでいないコミットを書くものであってはならない
# からだ。
update:
	git pull --ff-only
	$(MAKE) install

uninstall:
	@$(MAKE) --no-print-directory uninstall-binary INSTALL_DIR="$(INSTALL_DIR)"

uninstall-binary:
	@if [ -d "$(INSTALL_DIR)/sshc" ]; then \
		echo "sshc: uninstall destination is a directory: $(INSTALL_DIR)/sshc" >&2; \
		exit 1; \
	fi
	rm -f "$(INSTALL_DIR)/sshc"
	@echo "removed $(INSTALL_DIR)/sshc"
