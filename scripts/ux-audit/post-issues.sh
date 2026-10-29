#!/bin/sh
# docs/ux-audit.md の各項目（UX-NN）を GitHub のイシューにする。先にまとめのイシューを作り、各項目の本文からその番号を指す。
# 最後に、まとめの本文を各イシューの番号つきの一覧に置き換える。
# 使い方: sh scripts/ux-audit/post-issues.sh <本文の出力先>
set -eu
out=$1
repository=aida0710/sshc
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$out"

if [ -f "$out/summary-number" ]; then
  summary=$(cat "$out/summary-number")
else
  printf '%s\n' "作成中です。各項目のイシューを登録したあと、一覧に置き換えます。" > "$out/summary-placeholder.md"
  url=$(gh issue create --repo "$repository" --title "UI/UX監査（2026-10-30）: 34件の改善案のまとめ" --label enhancement --body-file "$out/summary-placeholder.md")
  summary=${url##*/}
  printf '%s\n' "$summary" > "$out/summary-number"
fi
echo "summary #$summary"

python3 "$script_dir/make-issues.py" "$out" "$summary"

# 途中で止まっても、作ったものを作り直さないよう created.json に1件ずつ足す。
[ -f "$out/created.json" ] || printf '{}\n' > "$out/created.json"
python3 - "$out" <<'CREATE'
import json, subprocess, sys
from pathlib import Path
out = Path(sys.argv[1])
created = json.loads((out / "created.json").read_text())
for issue in json.loads((out / "issues.json").read_text()):
    if issue["number"] in created:
        continue
    title = f"{issue['title']}（{issue['number']}）"
    url = subprocess.run(
        ["gh", "issue", "create", "--repo", "aida0710/sshc", "--title", title, "--label", issue["label"], "--body-file", issue["body"]],
        check=True, capture_output=True, text=True,
    ).stdout.strip()
    created[issue["number"]] = url.rsplit("/", 1)[1]
    (out / "created.json").write_text(json.dumps(created, indent=2))
    print(issue["number"], url)
CREATE

# 全部の番号が分かったので、ほかの項目への参照をリンクにした本文で各イシューを書き直し、まとめを一覧にする。
python3 "$script_dir/make-issues.py" "$out" "$summary"
python3 - "$out" <<'RELINK'
import json, subprocess, sys
from pathlib import Path
out = Path(sys.argv[1])
created = json.loads((out / "created.json").read_text())
for issue in json.loads((out / "issues.json").read_text()):
    subprocess.run(["gh", "issue", "edit", created[issue["number"]], "--repo", "aida0710/sshc", "--body-file", issue["body"]], check=True, capture_output=True)
print("bodies relinked")
RELINK
gh issue edit "$summary" --repo "$repository" --body-file "$out/summary.md" >/dev/null
echo "summary updated"
