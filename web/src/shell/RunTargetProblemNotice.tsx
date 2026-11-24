import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { RemoteEngineStartCommands } from "../terminal/RemoteEngineStartCommands";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";

// 試作: 案A（いつも開いた形）と案B（畳んだ形）を localStorage の sshc-prototype-warning で切り替える。
function prototypeVariant(): "a" | "b" {
  try {
    return window.localStorage.getItem("sshc-prototype-warning") === "b" ? "b" : "a";
  } catch {
    return "a";
  }
}

// RunTargetProblemNotice は、リモートセッションの sshc エンジンを足す・外す・別タブで開くのに失敗した理由を、
// サイドバーの［リモートセッション］の上に出す。ターミナルの知らせのカード（NoticeFrame の ended）と同じ色で、
// 見出しは墨、説明は淡い字にし、赤は枠と地の色だけに使う。
export function RunTargetProblemNotice({ targets }: { targets: RunTargetsState }) {
  const t = useTranslate();
  const [variant] = useState(prototypeVariant);
  const [expanded, setExpanded] = useState(false);
  const about = targets.problemAbout;
  const notRunning = about?.code === "remote_engine_not_running";
  const title = notRunning ? t("runTarget.notRunningTitle") : targets.problem;
  const showCommands = notRunning && (variant === "a" || expanded);
  return (
    <section
      role="alert"
      className="relative mb-2 rounded-lg border border-danger/45 bg-[color-mix(in_srgb,var(--ui-danger)_9%,var(--ui-card))] p-2.5"
    >
      {/* 右上の［×］の分だけ、見出しと接続名の右を空ける。コマンドの行は幅いっぱいに使う。 */}
      <p className="break-words pr-5 text-xs font-semibold leading-5 text-ink mobile-layout:pr-9">{title}</p>
      {about === null || !notRunning ? null : (
        <p className="break-words pr-5 font-mono text-2xs leading-4 text-ink-muted mobile-layout:pr-9">{about.alias}</p>
      )}
      {showCommands ? <RemoteEngineStartCommands className="mt-2" /> : null}
      {about === null ? null : (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button small busy={targets.busy} onClick={targets.retryProblem}>{t("runTarget.tryAgain")}</Button>
          {notRunning && variant === "b" ? (
            <button
              type="button"
              aria-expanded={expanded}
              onClick={() => setExpanded(!expanded)}
              className="flex items-center gap-1 text-xs text-ink-muted underline underline-offset-2 hover:text-ink"
            >
              {t("runTarget.showHowToFix")}
              <Icon name="chevronRight" className={`size-3 transition-transform ${expanded ? "-rotate-90" : "rotate-90"}`} />
            </button>
          ) : null}
        </div>
      )}
      <button
        type="button"
        aria-label={t("terminal.noticeDismiss")}
        title={t("terminal.noticeDismiss")}
        onClick={targets.dismissProblem}
        className="absolute right-0.5 top-0.5 flex size-6 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink mobile-layout:size-10"
      >
        <Icon name="close" className="size-3.5" />
      </button>
    </section>
  );
}
