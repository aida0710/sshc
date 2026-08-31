# aida0710/homebrew-tap へ同期する Formula の正本。
# release workflow がタグと SHA-256 を更新して tap へ反映する。
# コミット済みの internal/ui/dist を含むソースから、Node.js なしでビルドする。
# macOS の os/user.Current() に必要なため CGO は無効化しない。
class Sshc < Formula
  desc "Manage OpenSSH configuration and connect from one window"
  homepage "https://github.com/aida0710/sshc"
  license "Apache-2.0"
  url "https://github.com/aida0710/sshc/archive/refs/tags/v0.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  head "https://github.com/aida0710/sshc.git", branch: "main"

  depends_on "go" => :build

  def install
    # ./cmd/sshc をビルドし、実際のリリースバージョンを埋め込む。
    # -s -w は std_go_args が追加するため重ねて指定しない。
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}"), "./cmd/sshc"
    generate_completions_from_executable(bin/"sshc", "completion")
  end

  test do
    # インストール済みバイナリのバージョンを検証する。
    assert_match "sshc #{version}", shell_output("#{bin}/sshc version")

    # engine が動作していない場合の終了コードとメッセージを検証する。
    output = shell_output("#{bin}/sshc status 2>&1", 1)
    assert_match "not running", output
    refute_match "no such file", output
  end
end
