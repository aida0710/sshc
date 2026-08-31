import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { IntegrationsApi, TerminalSettings } from "../api/integrations";
import type { Section } from "../routing/sectionRoute";
import type { WorkspaceRestoreRequest } from "../features/workspaces/TerminalWorkspace";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import type { TerminalSessionsState } from "./sessions";

type TerminalWorkspaceControllerOptions = {
  api: IntegrationsApi;
  consoles: TerminalSessionsState;
  enabled: boolean;
  section: Section | null;
  navigate: (section: Section) => void;
  closeNavigation: () => void;
};

export function useTerminalWorkspaceController({
  api,
  consoles,
  enabled,
  section,
  navigate,
  closeNavigation,
}: TerminalWorkspaceControllerOptions) {
  const [settings, setSettings] = useState<TerminalSettings>({});
  const [localShellProfiles, setLocalShellProfiles] = useState<
    Awaited<
      ReturnType<NonNullable<IntegrationsApi["localShellProfiles"]>>
    >["profiles"]
  >([]);
  const [activeConsole, setActiveConsole] = useState<string | null>(null);
  const [liveWorkspace, setLiveWorkspace] =
    useState<LiveWorkspaceSummary | null>(null);
  const [restoreRequest, setRestoreRequest] =
    useState<WorkspaceRestoreRequest | null>(null);
  const [consoleOrder, setConsoleOrder] = useState<string[]>([]);
  const restoreSequence = useRef(0);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void api
      .terminalSettings()
      .then((next) => {
        if (active) setSettings(next);
      })
      .catch(() => undefined);
    if (api.localShellProfiles !== undefined) {
      void api
        .localShellProfiles()
        .then((answer) => {
          if (active) setLocalShellProfiles(answer.profiles);
        })
        .catch(() => undefined);
    }
    return () => {
      active = false;
    };
  }, [api, enabled, section]);

  useEffect(() => {
    if (
      activeConsole !== null &&
      !consoles.sessions.some((session) => session.id === activeConsole)
    ) {
      setActiveConsole(null);
    }
  }, [consoles.sessions, activeConsole]);

  useEffect(() => {
    if (activeConsole !== null || consoles.sessions.length === 0) return;
    setActiveConsole(consoles.sessions[0]?.id ?? null);
  }, [consoles.sessions, activeConsole]);

  const showConsole = useCallback(
    (id: string) => {
      setActiveConsole(id);
      closeNavigation();
      navigate("Terminal");
      void consoles.refresh();
    },
    [closeNavigation, consoles, navigate],
  );

  const openWorkspace = useCallback(
    (id: string) => {
      restoreSequence.current += 1;
      setRestoreRequest({ id, sequence: restoreSequence.current });
      navigate("Terminal");
    },
    [navigate],
  );

  const consumeRestore = useCallback((sequence: number) => {
    setRestoreRequest((current) =>
      current?.sequence === sequence ? null : current,
    );
  }, []);

  const orderedConsoles = useMemo(() => {
    const rank = new Map(consoleOrder.map((id, index) => [id, index]));
    return consoles.sessions
      .map((session, index) => ({
        session,
        rank: rank.get(session.id) ?? consoleOrder.length + index,
      }))
      .sort((left, right) => left.rank - right.rank)
      .map((entry) => entry.session);
  }, [consoles.sessions, consoleOrder]);

  const openLocalShell = useCallback(
    async (profileId?: string) => {
      const opened = await consoles.open({
        kind: "shell",
        ...(profileId === undefined ? {} : { profileId }),
      });
      if (opened !== null) showConsole(opened.id);
    },
    [consoles, showConsole],
  );

  const duplicateConsole = useCallback(
    async (id: string) => {
      const session = consoles.sessions.find(
        (candidate) => candidate.id === id,
      );
      if (session === undefined) return null;
      const opened = await consoles.open(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias }
          : { kind: "shell" },
      );
      if (opened !== null) showConsole(opened.id);
      return opened;
    },
    [consoles, showConsole],
  );

  return {
    settings,
    localShellProfiles,
    activeConsole,
    liveWorkspace,
    restoreRequest,
    orderedConsoles,
    showConsole,
    openWorkspace,
    openLocalShell,
    duplicateConsole,
    consumeRestore,
    reorderConsoles: setConsoleOrder,
    setLiveWorkspace,
    setSettings,
  };
}
