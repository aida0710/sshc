# 署名と公証（macOS）

**この文書は「まだ買っていない」前提で書いてある。** 仕込みは済んでいるので、
secret を 5 つ入れれば次のリリースから署名と公証が走る。**workflow を書き換える
必要は無い。**

## いま何が起きているか

証明書が無いので、`desktop/adhoc.ts` が束に ad-hoc 署名を付けている。**配布署名の
代わりではない** ——Gatekeeper は ad-hoc を信頼しないので、利用者は初回に
「開発元を確認できません」を通ることになる。

それでも付けているのは、**封と中身の辻褄を合わせるため**である。electron-builder は
Electron の実行体を貰ってきて Info.plist も icon も resources も差し替えるので、
署名は前のものが残る。arm64 では署名の無い実行体はそもそも起動しない。

## 買ったあとにやること

### 1. Developer ID Application 証明書を作る

Apple Developer のアカウントで「Certificates → Developer ID Application」。
できた証明書を Keychain から **`.p12` として書き出す**（パスフレーズを付ける）。

```sh
base64 -i DeveloperID.p12 | pbcopy    # secret に貼る形にする
```

### 2. App Store Connect の API key を作る

App Store Connect → Users and Access → Integrations → App Store Connect API。
**役割は Developer で足りる。** ダウンロードできる `AuthKey_XXXXXXXX.p8` は
**一度しか落とせない。**

**Apple ID と app-specific password でも公証はできるが、API key を使う。**
漏れたときに渡るものが、アカウント全体ではなく、その鍵の権限に限られる。

### 3. secret を 5 つ入れる

`Settings → Secrets and variables → Actions`:

| 名前 | 中身 |
|---|---|
| `APPLE_CERTIFICATE_P12` | `.p12` の base64 |
| `APPLE_CERTIFICATE_PASSWORD` | `.p12` のパスフレーズ |
| `APPLE_API_KEY` | `AuthKey_XXXXXXXX.p8` の中身（そのまま貼る） |
| `APPLE_API_KEY_ID` | 鍵の ID（`AuthKey_` の後ろの 10 文字） |
| `APPLE_API_ISSUER` | Issuer ID（API key の画面の上にある UUID） |
| `APPLE_TEAM_ID` | Team ID（Membership の画面にある 10 文字） |

### 4. tag を打つ

それだけである。`APPLE_CERTIFICATE_P12` が空でなくなった時点で、
`CSC_IDENTITY_AUTO_DISCOVERY` が `true` に変わり、`adhoc.ts` は自分の出番が
終わったと判断して何もしなくなる。公証は `APPLE_API_*` が揃っていれば
electron-builder が走らせる。

## 済んだあとに消せるもの

- `desktop/adhoc.ts` と、`build.afterPack` の指定
- `docs/release-install.md` の macOS の項（「このまま開く」の案内）
- 上の記述に依存しているテスト

**すぐには消さない。** 公証した束が実機で警告なしに開くことを一度確かめてから、
まとめて落とす方がよい——戻す必要が出たときに、戻す先が残っている。

## 確かめ方

公証まで通った束は、ダウンロードした機械で次が通る。

```sh
spctl --assess --type execute -vv /Applications/sshc.app
# → accepted, source=Notarized Developer ID
xcrun stapler validate /Applications/sshc.app
```

**この検査は実機でしかできない。** CI が言えるのは「公証の段が成功で終わった」
までである。
