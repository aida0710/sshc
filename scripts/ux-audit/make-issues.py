"""docs/ux-audit.md の各項目（### UX-NN）から、GitHub のイシューの本文を作る。

使い方: python3 scripts/ux-audit/make-issues.py <出力先> <まとめのイシューの番号>
出力: <出力先>/UX-NN.md（本文）と <出力先>/issues.json（番号・タイトル・ラベル・本文のファイル・貼る画像）。
<出力先>/created.json（UX-NN からイシューの番号）があれば、まとめのイシューの本文 <出力先>/summary.md も作る。
画像は issue-assets ブランチの ux-audit-2026-10-30/ に置く前提で、raw の URL で貼る。
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AUDIT = ROOT / "docs" / "ux-audit.md"
IMAGE_BASE = "https://raw.githubusercontent.com/aida0710/sshc/issue-assets/ux-audit-2026-10-30"
SOURCE_COMMIT = "c68affe42"

TITLES = {
    "UX-01": "画面の組み方: タブレット縦（600〜1023pxのタッチ）がスマホの組み方のまま横に伸びる",
    "UX-02": "画面の組み方: スマホ横で上下の帯が高さを使い、中身が約120pxしか残らない",
    "UX-03": "Connections・Snippets: 選ぶ前は詳細の大きな領域が空き、一覧の列は狭くて名前が切れる",
    "UX-04": "広い画面: 板が中央の最大幅に収まって左右が空き、Settingsのナビが空きに浮く",
    "UX-05": "SSH Config: Includeの木でファイル名が切れ、パターンが単語の途中で折り返す",
    "UX-06": "SFTP: 一覧が読みにくい（スマホは権限とUIDが先、PCは列が離れる、画像・動画の区別が無い）",
    "UX-07": "ファイルのプレビュー: スマホで画面の中の小さなダイアログになり、画像が上に寄って下が空く",
    "UX-08": "Diagnostics: 接続名が自由入力で例の文字が値に見え、設定ファイルが絶対パスで長い",
    "UX-09": "Home: カードに「グループなし」の札が出て高さがそろわず、検索の例の文字が切れる",
    "UX-10": "Settings > Terminal: 設定の組の帯がスマホで3段に折り返す",
    "UX-11": "SSH Keys: 作成の入口が二重で、ssh-agentの警告が両方のOSの手順を出す",
    "UX-12": "Known Hosts: スマホのカードが1件4行で長く、ハッシュ化したエントリがどの接続先か分からない",
    "UX-13": "Connections: 接続先の編集のタブがスマホで横にはみ出し、続きがある印が無い",
    "UX-14": "サイドバー: 接続できなかったセッションの2行目が「接続できませんでした。理由はター…」と切れる",
    "UX-15": "タッチだけの大きい画面: タブレット横・2in1のタブレットの姿勢で20pxの押す場所が残る（着手の前に判断が要る）",
    "UX-16": "ターミナル: 文字の大きさをその場で変えられない（ピンチ・Ctrl＋ホイール・Ctrl+=／−／0）",
    "UX-17": "ファイルのプレビュー: 指の操作（左右で前後、下で閉じる）、スマホの全画面、SFTPの詳細の前へ・次へが無い",
    "UX-18": "SFTP: 画像・動画のサムネイルと、並べて見る表示が無い",
    "UX-19": "SFTP: 左右2つのペイン（ローカル＋リモート）にするのに4回の操作が要る",
    "UX-20": "SFTP: 空の状態から接続するのに2回の操作が要り、未接続でもパスの帯が出る",
    "UX-21": "Menu: スマホで22項目を探す検索が上に無い",
    "UX-22": "キーボードショートカット: 割り当てられる操作が10種しか無い",
    "UX-23": "Home: カードに長押し（右クリック）のメニューが無い",
    "UX-24": "常に出る長い説明: Vaultの作成・ロック解除のサインアウトの2文と、Syncの4行",
    "UX-25": "ターミナル: 幅の狭いブラウザで同じセッションを開くと、打ったコマンドの行の一部が消える",
    "UX-26": "Home: ふつうのHost *に対して「2件の確認が必要です」を出し続け、行き先には何も出ない",
    "UX-27": "スマホの下のバー: Menuから開いたページのとき、どれも選ばれない",
    "UX-28": "スマホ: ［新しいセッション］と実行先の選択を開くと検索欄にフォーカスが入り、キーボードが開く",
    "UX-29": "Diagnostics: 設定ファイルの一覧で状態の点が潰れ、長いパスが右で切れる",
    "UX-30": "Home: 確認の帯のボタンが文の右にある（知らせのボタンは文の下に置く決まり）",
    "UX-31": "History: 復元の知らせに履歴IDが出る",
    "UX-32": "Settings: 設定の確定の仕方がページごとに違う（すぐ保存と［保存］が混ざる）",
    "UX-33": "検索（Ctrl+K）: 結果が一致の度合いで並ばない",
    "UX-34": "ファイルのプレビュー: 動画・音声のファイルが無いときも「このブラウザでは再生できない形式です」と出る",
}

# 不具合の項目だけに付ける「確かめ方」。直したあとに何を見れば直ったと言えるか。
CHECKS = {
    "UX-25": "1920幅のブラウザで画面から開いたローカルシェルに、印を付けた長い行（`echo _00_abcdefghij_01_…`）を打つ。同じセッションを1366幅・390幅のブラウザで開き、打った行の文字が欠けないこと。`web/src/terminal/screenSnapshot.test.ts`に、幅の境で行を編集したシェルの行を狭い幅で開く場面を足す。",
    "UX-26": "`Host *`だけを書いた設定で、Homeに確認の帯が出ないこと。読めないIncludeなど直すべき問題があるときは、帯の文に何について・どこでが入り、［設定を確認］の行き先にその問題が出ること。",
    "UX-27": "スマホの幅でMenu > SSH Keysを開いたとき、下のバーのMenuが`aria-current=\"page\"`になること。上のバーの左からMenuへ戻れること。",
    "UX-28": "スマホの幅で［新しいセッション］と実行先の選択を開いたとき、検索欄にフォーカスが入らないこと（`document.activeElement`が［閉じる］）。",
    "UX-29": "360px幅で、設定ファイルの一覧の状態の点が6pxの丸のまま残り、長いパスが折り返して横にはみ出さないこと。",
    "UX-30": "Homeの確認の帯で、ボタンが文の下に並ぶこと（共通の`Notice`の`actions`を使う）。",
    "UX-31": "Historyから復元したときの知らせに履歴IDが出ないこと。",
    "UX-34": "存在しない`.mp4`のパスをターミナルに出して押したとき、見つからないという理由が出ること。再生できない形式の動画（Chromiumで開くH.265など）では、今までどおり形式の理由が出ること。",
}

BUGS = {"UX-25", "UX-26", "UX-27", "UX-28", "UX-29", "UX-30", "UX-31", "UX-34"}

# 公開のイシューでは、手元の記録や会話の中だけで通じる言い方を、読めば分かる言い方に替える。
REWRITES = [
    ("（worklog 2026-10-29 16:49）", "（#501）"),
    ("10-28 Dの再掲", "2026-10-28の点検の再掲"),
    ("10-28 Dの", "2026-10-28の点検の"),
    ("（10-28 Dの", "（2026-10-28の点検の"),
    ("10-28の点検の`shoot.mjs`", "以前の点検の撮影スクリプト"),
    ("、`screens/setup.json`の`attention`", ""),
    ("、`screens/setup-run3.json`の`attention`", ""),
    ("開き直すたびに崩れ方が変わりました（下の2枚）。", "開き直すたびに崩れ方が変わりました。"),
    ("manifest.json（tablet-landscape・2in1-tabletの`smallTargets`）、", ""),
    ("manifestで数えると、", "撮影のときに数えると、"),
    ("2026-10-10の記録（sshcのすすめ「撮影結果と横向きで見つかった表示制約」）で既に指摘され、直していないままです。", "2026-10-10の確認でも同じことを見つけていて、まだ直していません。"),
    ("この方針は「タッチ画面付きのPCでも画面がPCの大きさならPCと同じにする」という相談で決めたもので、", "この方針は、タッチ画面付きのPCの表示を決めたときのもので、"),
    ("方針L57により、", "画面の方針（タッチ対応のPCでも、画面がPCの大きさならPCと同じ表示にする）により、"),
    ("方針L8（済んだ結果を出し続けない）", "画面の方針（済んだ操作の結果は出し続けない）"),
    ("（方針L10・L32）", "（常に出る長い説明は置かず、知らないと困ることだけを短く残す方針）"),
    ("（方針L101「tmuxのようになってほしい」）", "（tmuxのように、どのマシンから開いても同じ画面になってほしいという要望。#501）"),
    ("（方針L17）", "（警告には何について・どこでを入れる決まり。docs/writing-style.md）"),
    ("方針L98（スマホで勝手にキーボードを開かない）と同じ考え方です。", "スマホで勝手にキーボードを開かない方針（#494）と同じ考え方です。"),
    ("方針L10は「知らせに付ける操作ボタンは知らせの枠の中、文の下」です。", "知らせに付ける操作ボタンは、知らせの枠の中の文の下に置く決まりです。"),
    ("- **方針との関係**: L57の範囲を変える話なので、進めてよいかを決めてください。", "- **判断が要ること**: 上の方針の範囲を変える話なので、着手の前に進めるかを決めます。"),
]

# 公開しない画像（撮影のマシンのユーザー名が写る最初の撮影）を指す行は、文だけに置き換える。
LINE_REWRITES = [
    (re.compile(r"^  - 同じセッションでも開くたびに崩れ方が変わる例.*$", re.M),
     "  - 同じセッションでも、開き直すたびに崩れ方が変わりました（3つ目の出力が途中で途切れる回と、丸ごと無い回があった）。この画像は撮影のマシンのユーザー名が写るので貼っていません。"),
]

# まとめの結論のうち、手元の会話や記録だけで通じる言い方を言い換える。
SUMMARY_REWRITES = [
    ("（UX-15。方針L57と関わるので判断がほしいです）", "（UX-15。今の画面の方針の範囲を変えるので、着手の前に判断が要ります）"),
    ("拡大とプレビューのアイコン化は #509（別のセッション、作業中）が扱っています。切断の表示は別のセッション（sshc-46）が調べて直しています。", "プレビューの拡大とアイコン化は #509 が扱っています。切断したときの表示が同じことを2回言う件は、別に調べて直しているところです。"),
    ("拡大とプレビューのアイコン化は#509（別のセッション、作業中）が扱っています。切断の表示は別のセッション（sshc-46）が調べて直しています。", "プレビューの拡大とアイコン化は#509が扱っています。切断したときの表示が同じことを2回言う件は、別に調べて直しているところです。"),
]

IMAGE_PATTERN = re.compile(r"`((?:screens|sheets|sheets-extra)/[^`]+\.(?:png|json))`")


def published_name(path: str) -> str:
    directory, name = path.split("/", 1)
    return name if directory == "screens" else f"{directory}--{name}"


def sections(text: str) -> dict[str, list[str]]:
    found: dict[str, list[str]] = {}
    current = None
    for line in text.split("\n"):
        heading = re.match(r"^### (UX-\d+) ", line)
        if heading:
            current = heading.group(1)
            found[current] = []
            continue
        if line.startswith("## ") or line.startswith("### "):
            current = None
            continue
        if current is not None:
            found[current].append(line)
    return found


def link_others(text: str, number: str, created: dict[str, str]) -> str:
    """ほかの項目を指す「UX-NN」を、そのイシューへのリンクにする（自分の番号はそのまま）。"""
    def replace(match: re.Match[str]) -> str:
        other = match.group(0)
        return other if other == number or other not in created else f"#{created[other]}（{other}）"
    return re.sub(r"UX-\d\d", replace, text)


def body_for(number: str, lines: list[str], summary: str, created: dict[str, str]) -> tuple[str, list[str]]:
    images: list[str] = []
    kept: list[str] = []
    for line in lines:
        refs = [ref for ref in IMAGE_PATTERN.findall(line) if ref.endswith(".png")]
        images.extend(ref for ref in refs if ref not in images)
        stripped = IMAGE_PATTERN.sub("", line)
        # 画像のパスだけの行（「- `screens/a.png`、`…`」）は、下の画像の節に任せて消す。
        if refs and re.fullmatch(r"\s*-?\s*[、・\s]*(と|の)?[、・\s]*", stripped):
            continue
        kept.append(IMAGE_PATTERN.sub(lambda match: published_name(match.group(1)), line))
    text = "\n".join(kept).strip()
    for old, new in REWRITES:
        text = text.replace(old, new)
    for pattern, replacement in LINE_REWRITES:
        text = pattern.sub(replacement, text)
    text = link_others(text, number, created)
    parts = [
        f"UI/UX監査（2026-10-30、main `{SOURCE_COMMIT}`）の{number}です。一覧と調べ方は #{summary} にあります。",
        "",
        text,
    ]
    if number in CHECKS:
        parts += ["", "## 確かめ方", "", CHECKS[number]]
    if images:
        parts += ["", "## 画像", ""]
        for image in images:
            name = published_name(image)
            parts += [f"`{name}`", "", f"![{name}]({IMAGE_BASE}/{name})", ""]
        parts += ["撮影: 架空のデータ（接続先18件、使い捨てのsshd）、Chromium（ヘッドレス、`--disable-webgl`）、日本語、ダーク。ファイル名の先頭はフォームファクタ（phone 390×844、tablet 820×1180、2in1 1368×912（主はマウス、タッチもある）、desktop 1920×1080、laptop 1366×768、phone-landscape 844×390、tablet-landscape 1180×820、2in1-tablet 1368×912（タッチだけ））。`sheets--`は4つのフォームファクタを横に並べたもの。"]
    return "\n".join(parts).rstrip() + "\n", images


def chapter(text: str, heading: str) -> str:
    """text の中の「## heading」から次の「## 」までを返す。"""
    start = text.index(f"## {heading}")
    end = text.find("\n## ", start + 1)
    return text[start:end if end >= 0 else len(text)].split("\n", 1)[1].strip()


def summary_for(created: dict[str, str]) -> str:
    """まとめのイシューの本文。created は UX-NN からイシューの番号への対応。"""
    text = AUDIT.read_text()
    rows = []
    for line in chapter(text, "3. 一覧").split("\n"):
        match = re.match(r"^\| (UX-\d+) \| (.+?) \| (.+?) \| .+? \| .+? \| (.+?) \| (.+?) \|$", line)
        if match and match.group(1) in created:
            number, kind, summary, priority, size = match.groups()
            rows.append(f"- [ ] #{created[number]} {number} {summary}（{kind}、優先度 {priority}・規模 {size}）")
    conclusion = chapter(text, "結論")
    for old, new in SUMMARY_REWRITES:
        conclusion = conclusion.replace(old, new)
    conclusion = link_others(conclusion, "", created)
    references = chapter(text, "2. 外部の参考事例")
    for number, issue in created.items():
        references = references.replace(number, f"#{issue}")
    parts = [
        f"sshcの画面を、スマホ・タブレット・2in1・デスクトップの4つのフォームファクタ（ほかに重い画面だけスマホ横・タブレット横・2in1のタブレットの姿勢・1366×768）で撮って見直し、改善案を{len(rows)}件のイシューにしました。対象はmainの`{SOURCE_COMMIT}`です。",
        "",
        "## 結論",
        "",
        conclusion,
        "",
        "## 一覧",
        "",
        "優先度は高・中・低、規模は小（1日以内）・中（数日）・大（1週間以上、設計から）の目安です。",
        "",
        *rows,
        "",
        "## 調べ方",
        "",
        "- 架空のデータ（接続先18件、グループ3つと入れ子1つ、セッション5〜6、Vaultの6件、スニペット4件、鍵3つ）を入れた一時のHOMEでsshc engineを動かし、Playwright（Chromium、ヘッドレス、`--disable-webgl`、日本語、ダーク）で撮りました。SSHとSFTPの接続先は使い捨てのsshdのコンテナです。",
        "- 2in1のキーボードを付けた姿勢（主はマウス、タッチもある）は、Playwrightの`hasTouch`では作れないので、Chromiumの起動設定`--blink-settings=primaryPointerType=4,availablePointerTypes=6,primaryHoverType=2,availableHoverTypes=3`で作りました。指のタップはCDPの`Input.dispatchTouchEvent`で送りました。",
        "- 撮るたびに、横のはみ出し、小さい押す場所、切れた文字、主の領域の幅を数値で残しました。",
        "- 確かめていないもの: iPadのSafari、AndroidのAPK（WebView）、実機の2in1、ライトの配色。",
        "",
        "## 外部の参考事例",
        "",
        references,
    ]
    return "\n".join(parts).rstrip() + "\n"


def main() -> None:
    out = Path(sys.argv[1])
    summary = sys.argv[2]
    out.mkdir(parents=True, exist_ok=True)
    found = sections(AUDIT.read_text())
    created_path = out / "created.json"
    created = json.loads(created_path.read_text()) if created_path.exists() else {}
    issues = []
    for number, title in TITLES.items():
        body, images = body_for(number, found[number], summary, created)
        (out / f"{number}.md").write_text(body)
        issues.append({
            "number": number,
            "title": title,
            "label": "bug" if number in BUGS else "enhancement",
            "body": str(out / f"{number}.md"),
            "images": images,
        })
    (out / "issues.json").write_text(json.dumps(issues, ensure_ascii=False, indent=2))
    if created:
        (out / "summary.md").write_text(summary_for(created))
    print(len(issues), "issues")


if __name__ == "__main__":
    main()
