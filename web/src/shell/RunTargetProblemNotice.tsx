import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { RemoteEngineStartCommands } from "../terminal/RemoteEngineStartCommands";
import { Button, Notice } from "../ui/surface";

// RunTargetProblemNotice は、リモートセッションの sshc エンジンについての失敗（足せない、状態を読めないなど）を知らせる。
// 接続先で sshc エンジンが動いていないときは、見出しと接続名だけを見せ、接続先で打つコマンドは［直し方］に畳む。
// サイドバーの高さを取らないため（利用者が畳んだ形を選んだ）。
export function RunTargetProblemNotice({ problem, showAlias = true, onRetry, onDismiss }: {
  problem: { message: string; code: string; alias: string };
  // showAlias は、接続名を見出しの下に出すか。置いた場所の見出しが接続名を言うときは出さない。
  showAlias?: boolean;
  // onRetry は、失敗した操作のやり直し。あると［もう一度試す］を出し、終わるまで待ちの印を出す。
  onRetry?: (() => Promise<void>) | undefined;
  onDismiss?: (() => void) | undefined;
}) {
  const t = useTranslate();
  const [retrying, setRetrying] = useState(false);
  const retry = onRetry === undefined ? undefined : (
    <Button
      small
      busy={retrying}
      onClick={() => {
        setRetrying(true);
        void onRetry().finally(() => setRetrying(false));
      }}
    >
      {t("runTarget.tryAgain")}
    </Button>
  );
  if (problem.code !== "remote_engine_not_running") {
    return <Notice tone="danger" compact actions={retry} onDismiss={onDismiss}>{problem.message}</Notice>;
  }
  return (
    <Notice
      tone="danger"
      compact
      title={t("runTarget.notRunningTitle")}
      details={{ label: t("runTarget.showHowToFix"), content: <RemoteEngineStartCommands /> }}
      actions={retry}
      onDismiss={onDismiss}
    >
      {showAlias ? <span className="font-mono text-2xs">{problem.alias}</span> : undefined}
    </Notice>
  );
}
