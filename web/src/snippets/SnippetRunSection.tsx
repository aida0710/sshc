import { useTranslate } from "../i18n/context";
import { PageSection } from "../ui/page";
import { Button } from "../ui/surface";
import type { Job, Preview } from "./api";
import { snippetProblemLabelKey, snippetStatusLabelKey } from "./snippetLabels";

// SnippetRunSection は、開いているスニペットを選んだホストでまとめて実行する節である。実行の前に、
// ホストごとに展開したコマンドと経路を確認させ、実行中と終わった後はホストごとの出力を並べる。
export function SnippetRunSection({
  aliases,
  targets,
  preview,
  job,
  previewing,
  running,
  cancelling,
  busy,
  onTargetsChange,
  onPreview,
  onRun,
  onCancel,
}: {
  aliases: string[];
  targets: string[];
  preview: Preview | null;
  job: Job | null;
  previewing: boolean;
  running: boolean;
  cancelling: boolean;
  // busy は、このページのどれかの操作を待っている間である。終わるまでボタンを止める。
  busy: boolean;
  onTargetsChange: (targets: string[]) => void;
  onPreview: () => void;
  onRun: () => void;
  onCancel: (jobId: string) => void;
}) {
  const t = useTranslate();
  return (
    <PageSection heading={t("snippets.runHeading")} meta={t("snippets.targets")}>
      <div role="group" aria-label={t("snippets.targets")} className="grid grid-cols-2 gap-x-4 gap-y-2 text-[13px] sm:grid-cols-3">
        {aliases.map((alias) => (
          <label key={alias} className="flex min-w-0 items-center gap-2 break-all text-ink">
            <input
              type="checkbox"
              className="size-[15px] shrink-0 accent-accent"
              checked={targets.includes(alias)}
              onChange={(event) =>
                onTargetsChange(event.target.checked ? [...targets, alias] : targets.filter((value) => value !== alias))
              }
            />
            {alias}
          </label>
        ))}
      </div>
      <Button className="mt-3" busy={previewing} disabled={busy || targets.length === 0} onClick={onPreview}>
        {t("snippets.preview")}
      </Button>
      {preview === null ? null : (
        <section className="mt-4 min-w-0 rounded-[10px] bg-notice p-4">
          <h4 className="text-sm font-medium">{t("snippets.confirm")}</h4>
          {preview.targets.map((target) => (
            <div key={target.targetId} className="mt-2">
              <p className="text-xs font-medium">
                {target.target.alias} · {target.target.user}@{target.target.hostName}:{target.target.port}
              </p>
              {(target.target.route ?? []).map((hop, index) => (
                <p key={`${hop.alias}-${index}`} className="mt-1 break-all text-xs text-notice-ink">
                  {index + 1}. {hop.user}@{hop.hostName}:{hop.port}
                  {hop.proxyCommand === undefined || hop.proxyCommand === "" ? "" : ` · ProxyCommand: ${hop.proxyCommand}`}
                </p>
              ))}
              <pre className="mt-1 overflow-auto rounded bg-code-bg p-2 text-xs text-code-fg">{target.command}</pre>
            </div>
          ))}
          <Button kind="primary" className="mt-3" busy={running} disabled={busy} onClick={onRun}>
            {t("snippets.run")}
          </Button>
        </section>
      )}
      {job === null ? null : (
        <section className="mt-4 border-t border-line pt-4">
          <div className="flex items-center">
            <h4 className="grow text-sm font-medium">
              {t("snippets.results")} · {t(snippetStatusLabelKey(job.status))}
            </h4>
            {job.status === "running" ? (
              <Button busy={cancelling} disabled={busy} onClick={() => onCancel(job.id)}>
                {t("snippets.cancel")}
              </Button>
            ) : null}
          </div>
          {job.results.map((result) => {
            const problemKey = result.status === "failed" ? snippetProblemLabelKey(result.problem) : undefined;
            return (
              <div key={result.targetId} className="mt-3">
                <p className="text-xs font-medium">
                  {result.alias} · {t(snippetStatusLabelKey(result.status))}
                  {result.exitCode === undefined ? "" : ` (${result.exitCode})`}
                  {problemKey ? ` · ${t(problemKey)}` : ""}
                </p>
                {result.stdout ? (
                  <pre className="mt-1 max-h-40 overflow-auto rounded bg-code-bg p-2 text-xs text-code-fg">{result.stdout}</pre>
                ) : null}
                {result.stderr ? (
                  <pre className="mt-1 max-h-40 overflow-auto rounded bg-danger/10 p-2 text-xs text-danger">{result.stderr}</pre>
                ) : null}
              </div>
            );
          })}
        </section>
      )}
    </PageSection>
  );
}
