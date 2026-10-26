import type { MouseEvent, ReactNode } from "react";
import type { RequestFailureDiagnostic } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sectionPath } from "../routing/sectionRoute";
import type { VaultRecheckPhase } from "../session/useAppSession";
import { secondaryAction } from "../ui/form";
import { EntryScreen } from "../ui/EntryScreen";
import { Icon } from "../ui/icons";
import { PageBoard } from "../ui/PageBoard";
import { Button, Card } from "../ui/surface";
import { ErrorDiagnosticNotice } from "./ErrorDiagnosticNotice";

function reloadHere() {
  window.location.replace(window.location.pathname + window.location.search);
}

// The engine could not be reached or refused the browser: nothing else is
// usable, so the page is the error and a way to try again.
export function BootstrapErrorScreen({ failure, requestFailure, version, onCloseFailure }: {
  failure: string;
  requestFailure: RequestFailureDiagnostic | null;
  version: string;
  onCloseFailure: () => void;
}) {
  const t = useTranslate();
  return (
    <EntryScreen heading={t("shell.title")} description={<p role="alert" className="text-danger">{t("shell.bootstrapFailed")}</p>}>
      <div className="flex flex-col gap-3.5">
        {requestFailure === null ? null : (
          <ErrorDiagnosticNotice diagnostic={requestFailure} version={version} onClose={onCloseFailure} />
        )}
        {failure === "" ? null : <CommandBlock>{failure}</CommandBlock>}
        <Button kind="primary" className="h-[38px] w-full text-sm" onClick={reloadHere}>
          {t("shell.bootstrapRetry")}
        </Button>
      </div>
    </EntryScreen>
  );
}

// The engine forgot this browser (it restarted, or the session was closed
// elsewhere). Reloading registers again; if the registration itself is gone,
// running sshc in a terminal opens a new one.
export function SessionEndedScreen() {
  const t = useTranslate();
  return (
    <EntryScreen heading={t("shell.sessionEndedHeading")} description={<p role="alert">{t("shell.sessionEnded")}</p>}>
      <div className="flex flex-col gap-3.5">
        <CommandBlock>$ sshc</CommandBlock>
        <Button kind="primary" className="h-[38px] w-full text-sm" onClick={reloadHere}>
          {t("shell.sessionReload")}
        </Button>
      </div>
    </EntryScreen>
  );
}

// CommandBlock shows a command or a raw failure in the monospace well of the design.
function CommandBlock({ children }: { children: ReactNode }) {
  return (
    <code className="block max-w-full overflow-x-auto rounded-md border border-line bg-well px-3 py-2 font-mono text-[12.5px] text-ink">
      {children}
    </code>
  );
}

// Covers the app while the vault's state is being confirmed after a
// foreground return, so that nothing is typed into a screen about to lock.
export function VaultRecheckOverlay({ phase }: { phase: Exclude<VaultRecheckPhase, "idle"> }) {
  const t = useTranslate();
  return (
    <div className="fixed inset-0 z-[100] grid min-h-screen place-items-center bg-canvas p-6">
      <Card
        as="section"
        role="status"
        className="w-full max-w-md p-6 sm:p-8"
      >
        <h1 className="text-lg font-semibold">{t("shell.title")}</h1>
        <p className="mt-3 text-sm leading-6 text-ink-muted">
          {t(phase === "checking" ? "shell.vaultChecking" : "shell.vaultCheckRetrying")}
        </p>
      </Card>
    </div>
  );
}

export function NotFoundSection({ pathname, onGoHome }: {
  pathname: string;
  onGoHome: (event: MouseEvent<HTMLAnchorElement>) => void;
}) {
  const t = useTranslate();
  return (
    <div className="h-full overflow-y-auto">
      <PageBoard labelledBy="not-found-heading" className="grid place-items-center">
        <div className="flex max-w-md flex-col items-center gap-1.5 text-center">
          <span aria-hidden="true" className="mb-2 grid size-11 place-items-center rounded-xl bg-well text-ink-muted">
            <Icon name="pageNotFound" className="size-[22px]" />
          </span>
          <h2 id="not-found-heading" className="text-lg font-semibold text-ink">{t("shell.pageNotFound")}</h2>
          <p className="text-[13px] text-ink-muted">{t("shell.pageNotFoundDescription")}</p>
          <code className="mt-2.5 rounded-md border border-line bg-well px-3 py-2 font-mono text-[12.5px] text-ink">{pathname}</code>
          <a href={sectionPath("Home")} onClick={onGoHome} className={`${secondaryAction} mt-2.5 inline-flex min-h-11 items-center gap-1.5 md:min-h-0`}>
            <Icon name="home" className="size-[15px]" />
            {t("shell.goHome")}
          </a>
        </div>
      </PageBoard>
    </div>
  );
}
