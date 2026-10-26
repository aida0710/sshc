# aida0710/homebrew-tap へ同期する Formula の正本。
# release workflow がタグと SHA-256 を更新して tap へ反映する。
# 画面（internal/ui/dist/app）はリポジトリに入っていないので、Node.js で組み立ててからビルドする。
# macOS の os/user.Current() に必要なため CGO は無効化しない。
class Sshc < Formula
  desc "Manage OpenSSH configuration and connect from one window"
  homepage "https://github.com/aida0710/sshc"
  license "Apache-2.0"
  url "https://github.com/aida0710/sshc/archive/refs/tags/v0.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  head "https://github.com/aida0710/sshc.git", branch: "main"

  # タグのソースアーカイブには .git が無く、Go がコミットとコミット日時
  # （vcs.revision・vcs.time）を記録できない。release workflow がタグのコミットと
  # その日時（UTC の RFC 3339）を書き込み、ビルドで渡す。
  SOURCE_REVISION = "".freeze
  SOURCE_COMMIT_TIME = "".freeze

  depends_on "go" => :build
  # 画面は CI・リリースと同じ Node 22 で組み立てる。Node が違うと組み立ての結果が変わる。
  depends_on "node@22" => :build

  def install
    # 埋め込む画面を組み立てる。npm は web/ の中で走らせる（--prefix は下位命令ごとに意味が違う）。
    # node@22 は keg-only なので、その bin を PATH の先頭に置く。
    ENV.prepend_path "PATH", Formula["node@22"].opt_bin
    cd "web" do
      system "npm", "ci", *std_npm_args(prefix: false)
      system "npm", "run", "build"
    end

    # ./cmd/sshc をビルドし、実際のリリースバージョンを埋め込む。
    # -s -w は std_go_args が追加するため重ねて指定しない。
    system "go", "build", *std_go_args(ldflags: build_ldflags), "./cmd/sshc"
    generate_completions_from_executable(bin/"sshc", "completion")
  end

  # HEAD のビルドは git の作業木から Go がコミットを記録するので、タグのコミットを渡さない。
  def build_ldflags
    flags = ["-X main.version=#{release_version}"]
    unless version.head?
      flags << "-X main.revision=#{SOURCE_REVISION}"
      flags << "-X main.commitTime=#{SOURCE_COMMIT_TIME}"
    end
    flags.join(" ")
  end

  # リリースの成果物と make build が埋め込むのと同じ、タグの名前（v0.41.0）を返す。
  # Homebrew の version はタグから v を除いた値（0.41.0）なので付け直す。
  # HEAD のビルドにはタグが無いので、Homebrew の version（HEAD-<commit>）のままにする。
  def release_version
    version.head? ? version.to_s : "v#{version}"
  end

  test do
    # インストール済みバイナリのバージョンを検証する。
    assert_match "sshc #{release_version}", shell_output("#{bin}/sshc version")

    # engine が動作していない場合の終了コードとメッセージを検証する。
    output = shell_output("#{bin}/sshc status 2>&1", 1)
    assert_match "not running", output
    refute_match "no such file", output
  end
end
