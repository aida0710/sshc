# これは tap `aida0710/homebrew-tap` に置く formula の正本である。
#
# **tap の名前は project 名ではない。** ひとつの tap に formula は何個でも入り、
# GUI 用の Casks/ も同居できる。HashiCorp も GoReleaser も Charm も `homebrew-tap`
# ひとつに全部置いており、利用者から見えるのは `brew install aida0710/tap/sshc`
# である——**次に何かを配るときに、また tap を作らずに済む。**
#
# **ここが原本で、tap にあるのは写しである。** リリースのたびに
# .github/workflows/release.yml が version と sha256 を差し替えて向こうへ押す。
# tap の側を手で直すと、次のリリースで上書きされる。
#
# **ソースからビルドする。** ビルド済みバイナリを落とすだけの formula でも
# 動きはするが、それができるのは `internal/ui/dist`（画面の束）がコミットされて
# いるからで、**npm も Node も要らずに `go build` が通る。** できるならその方が、
# 「brew が入れている」という利用者の期待と実際が一致する。
#
# macOS は CGO を切らない。**os/user.Current() があちらでは cgo を要る** ——
# Makefile の RELEASE_TARGETS が darwin を :1 にしているのと同じ理由である。
# std_go_args は CGO_ENABLED を触らないので、C コンパイラのある機械では既定で有効になる。
class Sshc < Formula
  desc "Manage OpenSSH configuration and connect from one window"
  homepage "https://github.com/aida0710/sshc"
  license "Apache-2.0"
  url "https://github.com/aida0710/sshc/archive/refs/tags/v0.0.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  head "https://github.com/aida0710/sshc.git", branch: "main"

  depends_on "go" => :build

  def install
    # **版を焼き込む。** cmd/sshc の main.version が既定の "dev" のままだと、
    # 走っているアプリと端末側の版が食い違ったときに、engine が出す理由が
    # 「dev と 0.1.0 は同じ版ではない」になる。
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}")
  end

  test do
    # **版を訊く道が、入ったものの上で通ることを見る。** ここが落ちるのは
    # ldflags を間違えたときと、コマンドを消したときである。
    assert_match "sshc #{version}", shell_output("#{bin}/sshc version")

    # **engine の居ない機械でも、答えは人が読めるものである。** brew の test は
    # 空の HOME で走るので、ここは必ず「engine が居ない」側を通る。黙って 0 を
    # 返さないことと、道の綴りを投げ返さないことの両方を見る。
    output = shell_output("#{bin}/sshc status 2>&1", 1)
    assert_match "not running", output
    refute_match "no such file", output
  end
end
