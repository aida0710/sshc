# これは tap `aida0710/homebrew-tap` の Casks/sshc.rb の正本である。
#
# **formula と同じ名前で同居できる。** `brew install aida0710/tap/sshc` は
# CLI（formula）を、`--cask` を付ければアプリを入れる。中身が違うので名前を
# 分ける方が親切に見えるが、**利用者が覚える名前は少ない方がよい。**
#
# **署名も公証もしていない。** ダウンロードしたものに macOS が付ける隔離の印は
# Homebrew も付けるので、初回は「開発元を確認できません」を通ることになる
# ——dmg を手で開いたときと同じである。
#
# **避ける手段は無い。** `--no-quarantine` は Homebrew 5.0 で非推奨、5.1 で
# 削除され、代替は用意されていない——Gatekeeper の迂回を提供しない、という
# 方針である。cask 側で打ち消すこともできない。
#
# 直すなら Developer ID を買って公証を通すしかない。それまでは、入れたあとに
# 一度だけシステム設定で許可してもらう（docs/release-install.md）。
cask "sshc" do
  arch arm: "arm64", intel: "x64"

  version "0.0.0"
  sha256 arm: "0000000000000000000000000000000000000000000000000000000000000000",
         intel: "0000000000000000000000000000000000000000000000000000000000000000"

  url "https://github.com/aida0710/sshc/releases/download/v#{version}/sshc-#{version}-mac-#{arch}.dmg",
      verified: "github.com/aida0710/sshc/"
  name "sshc"
  desc "Manage OpenSSH configuration and connect from one window"
  homepage "https://github.com/aida0710/sshc"

  app "sshc.app"

  # **CLI は formula の担当である。** アプリの中にも同じ実体が入っているが、
  # cask がそれを PATH へ張ると、brew が同じ名前を二度管理することになる。
  # 端末から使いたい人は `brew install aida0710/tap/sshc` を入れる。

  # **持ち物は消してよいが、鍵と設定は消さない。** ~/.ssh はこのアプリが作った
  # ものではない。zap が触るのは、このアプリが自分のために作った場所だけである。
  zap trash: [
    "~/Library/Application Support/sshc",
    "~/Library/Caches/com.github.aida0710.sshc",
    "~/Library/Preferences/com.github.aida0710.sshc.plist",
    "~/Library/Saved Application State/com.github.aida0710.sshc.savedState",
  ]
end
