import type { MouseEvent } from "react";
import type { RequestFailureDiagnostic } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sectionPath } from "../routing/sectionRoute";
import type { VaultRecheckPhase } from "../session/useAppSession";
import { secondaryAction } from "../ui/form";
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
    <div className="min-h-screen bg-canvas text-ink">
      {requestFailure === null ? null : (
        <ErrorDiagnosticNotice
          diagnostic={requestFailure}
          version={version}
          onClose={onCloseFailure}
        />
      )}
      <main className="flex flex-col items-start gap-3 p-6">
        <h1 className="text-base font-semibold">{t("shell.title")}</h1>
        <p role="alert" className="text-sm text-danger">
          {t("shell.bootstrapFailed")}
        </p>

        {failure === "" ? null : (
          <code className="max-w-full overflow-x-auto rounded-md border border-line bg-card px-2 py-1 text-xs">
            {failure}
          </code>
        )}

        <Button kind="primary" onClick={reloadHere}>
          {t("shell.bootstrapRetry")}
        </Button>
      </main>
    </div>
  );
}

// The engine forgot this browser (it restarted, or the session was closed
// elsewhere). Reloading registers again.
export function SessionEndedScreen() {
  const t = useTranslate();
  return (
    <main className="grid min-h-screen place-items-center bg-canvas p-6 text-ink">
      <Card as="section" className="flex w-full max-w-md flex-col items-start gap-4 p-6 sm:p-8">
        <h1 className="text-lg font-semibold">
          {t("shell.sessionEndedHeading")}
        </h1>
        <p role="alert" className="text-sm leading-6 text-ink-muted">
          {t("shell.sessionEnded")}
        </p>
        <Button kind="primary" onClick={reloadHere}>
          {t("shell.sessionReload")}
        </Button>
      </Card>
    </main>
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
    <div className="h-full overflow-y-auto p-4 md:p-5">
      <section
        aria-labelledby="not-found-heading"
        className="flex max-w-2xl flex-col gap-3"
      >
        <h2 id="not-found-heading" className="font-medium">
          {t("shell.pageNotFound")}
        </h2>
        <p className="text-sm text-ink-muted">
          {t("shell.pageNotFoundDescription")}
        </p>
        <code className="w-fit rounded-md border border-line bg-card px-2 py-1 text-sm">
          {pathname}
        </code>
        <a
          href={sectionPath("Home")}
          onClick={onGoHome}
          className={`${secondaryAction} w-fit`}
        >
          {t("shell.goHome")}
        </a>
      </section>
    </div>
  );
}
