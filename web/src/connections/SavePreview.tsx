import { tagClass } from "../ui/Tag";
import type { ConflictReport, DiffLine, FileDiff, SavePreview } from "../api/config";
import { ConfigDiagnosticList } from "../sshconfig/ConfigDiagnosticList";
import type { Problem } from "../api/client";
import { useTranslate } from "../i18n/context";
import { saveProblemMessage } from "../ui/saveProblemMessage";
import { Notice as Band } from "../ui/surface";
import { NoticeList } from "./NoticeList";

function DiffView({ lines }: { lines: DiffLine[] }) {
  return (
    <pre className="max-h-72 overflow-auto rounded-lg bg-tree p-3 font-mono text-xs leading-5">
      {lines.map((line, index) => (
        <div
          key={`${line.op}-${line.oldLine ?? 0}-${line.newLine ?? 0}-${index}`}
          className={
            line.op === "insert" ? "text-live" : line.op === "delete" ? "text-danger" : "text-ink-muted"
          }
        >
          {`${line.op === "insert" ? "+" : line.op === "delete" ? "-" : " "} ${line.text}`}
        </div>
      ))}
    </pre>
  );
}

function FileDiffView({ diff }: { diff: FileDiff }) {
  const t = useTranslate();
  return (
    <section className="flex flex-col gap-1">
      <h4 className="text-xs font-semibold text-ink-muted">
        {diff.path}
        {diff.created === true ? t("preview.newFile") : ""}
      </h4>
      {diff.truncated === true ? (
        <p className="text-xs text-notice-ink">{t("preview.tooLarge")}</p>
      ) : null}
      <DiffView lines={diff.lines} />
    </section>
  );
}

export function SavePreviewPanel({
  preview,
  conflict,
  problem,
}: {
  preview: SavePreview | null;
  conflict: ConflictReport | null;
  problem: Problem | null;
}) {
  const t = useTranslate();
  // 書き込んだものも、断られた理由も無いうちは何も出さない。いつも出ていると、済んだことのない操作の
  // 説明が画面に残り続ける。
  if (preview === null && conflict === null && problem === null) return null;
  return (
    <section aria-labelledby="preview-heading" className="flex flex-col gap-3 border-t border-line pt-5">
      <div className="flex items-center justify-between gap-3">
        <h3 id="preview-heading" className="text-sm font-medium text-ink">{t("preview.heading")}</h3>
        {preview === null ? null : (
          <span className={tagClass("neutral")}>
            {preview.diffs.length}
          </span>
        )}
      </div>

      {problem === null ? null : (
        <Band tone="danger">{saveProblemMessage(problem, t)}</Band>
      )}

      {problem?.diagnostics === undefined ? null : <ConfigDiagnosticList diagnostics={problem.diagnostics} />}

      {conflict === null ? null : (
        <div className="flex flex-col gap-2">
          <h4 className="text-xs font-semibold text-ink-muted">{t("preview.changedOnDisk")}</h4>
          <DiffView lines={conflict.externalChange} />
          <h4 className="text-xs font-semibold text-ink-muted">{t("preview.pendingChange")}</h4>
          <DiffView lines={conflict.localChange} />
          <p className="text-xs text-ink-muted">{t("preview.mergeByHand")}</p>
        </div>
      )}

      {preview === null ? null : (
        <div className="flex flex-col gap-3">
          {preview.diffs.map((diff) => (
            <FileDiffView key={diff.path} diff={diff} />
          ))}
          {(preview.effective ?? []).map((effective) => (
            <section key={effective.alias} className="flex flex-col gap-1">
              <h4 className="text-xs font-semibold text-ink-muted">{t("preview.explainedFor", { alias: effective.alias })}</h4>
              <ul>
                {effective.changes.map((change) => (
                  <li key={change.keyword} className="text-xs text-ink-muted">
                    {`${change.keyword}: ${change.before.join(", ") || t("preview.unset")} → ${
                      change.after.join(", ") || t("preview.unset")
                    }`}
                  </li>
                ))}
              </ul>
            </section>
          ))}
          <NoticeList notices={preview.notices ?? []} />
        </div>
      )}
    </section>
  );
}
