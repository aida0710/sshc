.PHONY: generate test deadcode build build-cli android-bind release-binaries release-cli-current fuzz e2e verify-generated verify-ui-dist integration integration-up integration-down integration-sshd-relax install install-binary uninstall uninstall-binary update

# FUZZTIME は target ごとの実行時間。長時間試す場合は、たとえば
# `make fuzz FUZZTIME=10m` のように上書きする。
FUZZTIME ?= 30s

# リポジトリ内のすべての fuzz target を package:Target の形で列挙する。target を
# 追加してここに加え忘れると TestMakefileFuzzTargetsCoverEveryFuzzFunction が失敗する。
FUZZ_TARGETS = \
	internal/config:FuzzParseRendersOriginalBytes \
	internal/config:FuzzExpandIncludePattern \
	internal/effective:FuzzParseValues \
	internal/effective:FuzzResolve \
	internal/terminal:FuzzAgentDecoderPreservesOrdinaryText \
	internal/knownhosts:FuzzParseKnownHostsRoundTrip \
	internal/acceptance:FuzzAPIRequestBodies \
	internal/remotesync:FuzzReadSnapshot


generate:
	go generate ./cmd/sshc
	go generate ./internal/api
	npm run generate:api --prefix web
	@# Go の検証規則と適合コーパスを web 用に生成する。規則の定義は Go 側に置く。
	go run ./internal/validate/cmd/rulegen .

test:
	go test -count=1 ./...
	go test -count=1 -race ./...
	@# Android 向けのビルドタグを検証する。実機向けビルドは CI の Android
	@# ジョブで gomobile と NDK を使って検証する。
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...
	$(MAKE) deadcode
	npm test --prefix web
	npm run typecheck --prefix web

# Linux、macOS、Windows のすべてで到達不能な関数を検出する。
# 実装は scripts/ci/deadcode.sh を参照。
deadcode:
	scripts/ci/deadcode.sh

fuzz:
	@set -e; for target in $(FUZZ_TARGETS); do \
		package="$${target%%:*}"; \
		name="$${target##*:}"; \
		echo "==> fuzz $$package $$name for $(FUZZTIME)"; \
		go test "./$$package" -run '^$$' -fuzz "^$$name$$" -fuzztime "$(FUZZTIME)"; \
	done

# E2E は時間がかかるため GitHub Actions では実行せず、必要なときにローカルで走らせる。
e2e: build
	npm run e2e --prefix web

# Web sourceから埋込み用の配布assetを再生成し、tracked fileの変更だけでなく
# hash名を持つ新しいuntracked assetも含めて、コミット済みの内容と比較する。
verify-ui-dist:
	npm run build --prefix web
	scripts/ci/check-ui-dist.sh

# APIモデルと埋込みUIを再生成し、コミット済みの生成物と異なれば失敗する。
# api/openapi.yaml をGoとTypeScriptの共通schemaとして扱う。
verify-generated: generate verify-ui-dist
	git diff --exit-code -- cmd/sshc/cli_contract.gen.go internal/api/models.gen.go \
		web/src/api/schema.d.ts web/src/api/validators.generated.ts \
		web/src/rules/generated.ts web/src/rules/corpus.generated.json

# VERSION を caller が渡した場合は専用の build channel へそのまま渡す。空なら
# helper が argv で git describe を実行し、exact tag がなければ dev を使う。

build:
	go run ./internal/nativebuild/cmd/nativebuild host-build --output-dir "bin"

# Android の名前解決には bionic の getaddrinfo を使用するため、AAR は
# gomobile と NDK を介して cgo を有効にしてビルドする。
#
# gomobile は go.mod の tool として固定してある。gobind は gomobile が PATH から
# 探すので、`go install golang.org/x/mobile/cmd/gobind@latest` を先に一度。
ANDROID_NDK_HOME ?= $(HOME)/Library/Android/sdk/ndk/28.2.13676358

# AAR 内の engine バージョンは gomobile の ldflags で設定する。
ANDROID_VERSION ?= dev

android-bind:
	@# gomobile は出力先を作成しないため、事前に用意する。
	mkdir -p android/app/libs
	ANDROID_NDK_HOME="$(ANDROID_NDK_HOME)" PATH="$(HOME)/go/bin:$$PATH" \
		go tool gomobile bind -target=android/arm64,android/amd64 -androidapi 26 \
		-ldflags "-X sshc/mobile.version=$(ANDROID_VERSION)" \
		-o android/app/libs/sshc.aar ./mobile

# build-cli は native runner と release job が共有する最小の CLI build primitive。
# target と output は caller の決定であり、暗黙の host 値や探索結果を使わない。
# CGO_ENABLED も target OS ごとの理由を知る caller が明示する。
build-cli:
	go run ./internal/nativebuild/cmd/nativebuild build

# UI を一度ビルドし、埋め込んだ Go バイナリを各ターゲット向けに生成する。
#
# macOS では `%u` と `%i` の展開に os/user.Current() を使うため cgo を有効にする。
# Linux では /etc/passwd を参照できるため CGO_ENABLED=0 とする。
#
# RELEASE_TARGETS は公開する CLI 成果物の一覧と一致させる。
RELEASE_TARGETS = darwin/arm64:1 darwin/amd64:1 linux/amd64:0 linux/arm64:0 windows/amd64:0 windows/arm64:0
RELEASE_CURRENT_ARCHES = amd64 arm64
RELEASE_DIR ?= dist

# Command-line Make variables are recursively expanded when referenced normally, and are
# automatically exported. Capture their raw bytes once with $(value ...), then export only
# dedicated names which cannot change the host go run target. Native recipes below contain
# fixed argv only; quotes, dollar signs, backticks and shell metacharacters never enter them.
override SSHC_NATIVE_VERSION := $(value VERSION)
override SSHC_NATIVE_GOOS := $(value GOOS)
override SSHC_NATIVE_GOARCH := $(value GOARCH)
override SSHC_NATIVE_CGO := $(value CGO_ENABLED)
override SSHC_NATIVE_OUTPUT := $(value OUTPUT)
override SSHC_NATIVE_RELEASE_TARGETS := $(value RELEASE_TARGETS)
override SSHC_NATIVE_RELEASE_ARCHES := $(value RELEASE_CURRENT_ARCHES)
override SSHC_NATIVE_RELEASE_DIR := $(value RELEASE_DIR)
export SSHC_NATIVE_VERSION SSHC_NATIVE_GOOS SSHC_NATIVE_GOARCH SSHC_NATIVE_CGO SSHC_NATIVE_OUTPUT
export SSHC_NATIVE_RELEASE_TARGETS SSHC_NATIVE_RELEASE_ARCHES SSHC_NATIVE_RELEASE_DIR

# Neutralize Go's target-selection inputs before go run compiles the host helper. Export
# canonical empty values instead of merely unexporting exact names, and keep the overrides
# target-specific so unrelated targets retain the developer's Go settings.
#
# This reaches only as far as Make's own variable table, which is keyed by exact spelling.
# The Windows port therefore keeps an inherited gOoS alongside our GOOS and exports both,
# and the case-insensitive process environment resolves that collision in the caller's
# favour. Windows CI showed this directly. What the child go build sees is settled one
# layer down instead, by withTargetEnvironment in nativebuild.go; a SSHC_NATIVE_* name that
# survives under the wrong spelling makes canonicalizeNativeEnvironment refuse to build.
override NATIVE_GO_RUN_TARGETS := build build-cli release-binaries release-cli-current
export GOENV GOOS GOARCH CGO_ENABLED
$(NATIVE_GO_RUN_TARGETS): override GOENV = off
$(NATIVE_GO_RUN_TARGETS): override GOOS =
$(NATIVE_GO_RUN_TARGETS): override GOARCH =
$(NATIVE_GO_RUN_TARGETS): override CGO_ENABLED =
unexport OUTPUT VERSION RELEASE_DIR RELEASE_TARGETS RELEASE_CURRENT_ARCHES

release-binaries:
	go run ./internal/nativebuild/cmd/nativebuild matrix

# release-cli-current は runner 自身の OS についてだけ standalone artifact を
# 作る。package job はこの後に同じ OS で smoke し、別 job が publish を集約する。
release-cli-current:
	go run ./internal/nativebuild/cmd/nativebuild release-current

# 統合テストはコンテナ内の S3 互換実装と OpenSSH sshd を使用する。
#
# 必要な環境変数がなければ統合テストはスキップされる。再現性を保つため、
# コンテナイメージは manifest digest で固定する。
S3_IMAGE   ?= chrislusf/seaweedfs@sha256:43b768cd62b00d132439cda881b93fd1adebf1b315e996e794087743821d771d
SSHD_IMAGE ?= linuxserver/openssh-server@sha256:96b9a4d3b5106746d08d43a6911650d4d21f7d5c7f2ac9660e792bdb5e63157c
S3_PORT    ?= 8333
SSHD_PORT  ?= 2222
S3_KEY     ?= SSHUITESTKEY
S3_SECRET  ?= sshuitestsecret
SSH_USER   ?= tester
SSH_PASS   ?= integration-only-password
SSH_DEST_USER ?= destination
SSH_DEST_PASS ?= integration-destination-password
SSH_KEY_PASSPHRASE ?= integration-key-passphrase
INTEGRATION_NETWORK ?= sshc-integration

integration-up:
	@printf '{"identities":[{"name":"sshc","credentials":[{"accessKey":"$(S3_KEY)","secretKey":"$(S3_SECRET)"}],"actions":["Admin","Read","Write","List","Tagging"]}]}' > .integration-s3.json
	docker rm -f sshc-s3 sshc-sshd sshc-sshd-destination >/dev/null 2>&1 || true
	docker network rm $(INTEGRATION_NETWORK) >/dev/null 2>&1 || true
	docker network create $(INTEGRATION_NETWORK) >/dev/null
	docker run -d --name sshc-s3 -p 127.0.0.1:$(S3_PORT):8333 \
		-v "$(PWD)/.integration-s3.json:/etc/seaweedfs/s3.json:ro" $(S3_IMAGE) \
		server -s3 -s3.port=8333 -s3.config=/etc/seaweedfs/s3.json -dir=/data
	docker run -d --name sshc-sshd --network $(INTEGRATION_NETWORK) \
		-p 127.0.0.1:$(SSHD_PORT):2222 \
		-e PASSWORD_ACCESS=true -e USER_NAME=$(SSH_USER) -e USER_PASSWORD=$(SSH_PASS) \
		$(SSHD_IMAGE)
	docker run -d --name sshc-sshd-destination --network $(INTEGRATION_NETWORK) \
		-e PASSWORD_ACCESS=true -e USER_NAME=$(SSH_DEST_USER) -e USER_PASSWORD=$(SSH_DEST_PASS) \
		$(SSHD_IMAGE)
	@rm -f .integration-key/id_integration .integration-key/id_integration.pub
	@mkdir -p .integration-key
	@ssh-keygen -q -t ed25519 -N "$(SSH_KEY_PASSPHRASE)" \
		-f .integration-key/id_integration -C sshc-integration
	@echo "waiting for the containers to answer"
	@ready=0; for i in $$(seq 1 60); do \
		if curl -sS -o /dev/null http://127.0.0.1:$(S3_PORT)/; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ready" != 1 ]; then \
		echo "SeaweedFS did not answer on port $(S3_PORT) within 60s" >&2; \
		docker logs sshc-s3 2>&1 | tail -100 >&2 || true; \
		exit 1; \
	fi
	@ready=0; for i in $$(seq 1 60); do \
		if ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null | grep -q .; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ready" != 1 ]; then \
		echo "sshd did not answer on port $(SSHD_PORT) within 60s" >&2; \
		docker logs sshc-sshd 2>&1 | tail -100 >&2 || true; \
		exit 1; \
	fi
	@ready=0; for i in $$(seq 1 60); do \
		if docker exec sshc-sshd ssh-keyscan -p 2222 sshc-sshd-destination 2>/dev/null | grep -q .; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ready" != 1 ]; then \
		echo "the destination sshd did not answer through the integration network within 60s" >&2; \
		docker logs sshc-sshd-destination 2>&1 | tail -100 >&2 || true; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory integration-sshd-relax
	@docker exec -i sshc-sshd sh -c ' \
		umask 077; \
		install -d -m 700 /config/.ssh; \
		touch /config/.ssh/authorized_keys; \
		chmod 600 /config/.ssh/authorized_keys; \
		cat >> /config/.ssh/authorized_keys' < .integration-key/id_integration.pub
	@ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null > .integration-proxy-known-hosts
	@docker exec sshc-sshd ssh-keyscan -p 2222 sshc-sshd-destination \
		2>/dev/null >> .integration-proxy-known-hosts

# OpenSSH 10 の PerSourcePenalties は、短時間に同一アドレスから接続を繰り返す
# 統合テストを途中で拒否する。テスト用コンテナ内だけで無効にし、連続接続で
# 設定結果を検証する。イメージ内の設定パスが変わる可能性があるため、利用可能な
# sshd_config を検索して更新する。
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
			grep -q "^MaxStartups" "$$configuration" || \
				printf "\nMaxStartups 64:64:64\n" >> "$$configuration"; \
			sed -i "s/^AllowTcpForwarding no$$/AllowTcpForwarding yes/" "$$configuration"; \
		done; \
		echo "PerSourcePenalties no; MaxStartups 64:64:64; AllowTcpForwarding yes ->" $$found'
	docker restart sshc-sshd
	@ready=0; for i in $$(seq 1 60); do \
		if ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null | grep -q .; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if [ "$$ready" != 1 ]; then \
		echo "sshd did not return after restart within 60s" >&2; \
		docker logs sshc-sshd 2>&1 | tail -100 >&2 || true; \
		exit 1; \
	fi
	@# Verify that repeated connections from one address are accepted.
	@for i in 1 2 3 4 5 6 7 8; do \
		ssh-keyscan -p $(SSHD_PORT) 127.0.0.1 2>/dev/null | grep -q . || { \
			echo "sshd refused connection $$i of 8 from one address: the per-source penalty is still on" >&2; \
			exit 1; \
		}; \
	done
	@echo "sshd accepts repeated connections from one address"

# 統合テスト用の資格情報と鍵は実行時に生成し、終了時に削除する。
integration-down:
	docker rm -f sshc-s3 sshc-sshd sshc-sshd-destination >/dev/null 2>&1 || true
	docker network rm $(INTEGRATION_NETWORK) >/dev/null 2>&1 || true
	rm -f .integration-s3.json
	rm -f .integration-proxy-known-hosts
	rm -f .integration-key/id_integration .integration-key/id_integration.pub
	rmdir .integration-key 2>/dev/null || true

# integration-up が起動した S3 互換サーバーと OpenSSH sshd に対して、条件付き
# PUT、SSH 認証、プロセス内 SSH クライアントとの相互運用を検証する。
# 外部サービスが必要なため `go test ./...` には含めない。
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
	SSHC_TEST_FORWARD_DEST_ADDR=sshc-sshd-destination:2222 \
	go test ./internal/objectstore ./internal/remotesync ./internal/sftp ./internal/sshdconformance -count=1 -v
	SSHC_TEST_PROXY_JUMP_ADDR=127.0.0.1:$(SSHD_PORT) \
	SSHC_TEST_PROXY_JUMP_USER=$(SSH_USER) \
	SSHC_TEST_PROXY_JUMP_PASSWORD=$(SSH_PASS) \
	SSHC_TEST_PROXY_DEST_ADDR=sshc-sshd-destination:2222 \
	SSHC_TEST_PROXY_DEST_USER=$(SSH_DEST_USER) \
	SSHC_TEST_PROXY_DEST_PASSWORD=$(SSH_DEST_PASS) \
	SSHC_TEST_PROXY_KNOWN_HOSTS="$(CURDIR)/.integration-proxy-known-hosts" \
	go test ./integration -run '^TestCLIUsesVaultPasswordsAcrossARealProxyJump$$' -count=1 -v

# バイナリは安定したパスへ置く。デスクトップ側はここへ symlink を張り、CLI と UI が
# 同じバイナリを使用する。別の場所でビルドし直すと
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
		*) echo "note: $(INSTALL_DIR) is not on PATH; add it to run 'sshc ssh <alias>' by name" ;; \
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
# 持たない環境でリリース済みバイナリを更新するためのものである。
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
