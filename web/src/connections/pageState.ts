import { useState } from "react";
import type { Problem } from "../api/client";
import type { SavePreview } from "../api/config";
import type { AdvancedArea, ConnectionPanel, ConnectionTarget } from "../routing/connectionRoute";
import type { HostSelection } from "./ConnectionTree";

// RefreshState は、保存したあとに保存済みの接続を読み直す状態である。refreshing と failed の
// あいだは、画面に出ている保存済みの値が古いかもしれないので、接続と編集を止める。
export type RefreshState = "idle" | "refreshing" | "failed";

export function useSelectionState(target: ConnectionTarget | null, invalid: boolean) {
  const [selection, setSelection] = useState<HostSelection | null>(
    target === null ? null : { path: target.path, alias: target.alias },
  );
  const [invalidLocation, setInvalidLocation] = useState(invalid);
  const [activePanel, setActivePanel] = useState<ConnectionPanel>(target?.panel ?? "Basic");
  const [activeAdvanced, setActiveAdvanced] = useState<AdvancedArea>(target?.advanced ?? "Jump");
  const [missingSelection, setMissingSelection] = useState(false);
  return {
    selection, setSelection,
    invalidLocation, setInvalidLocation,
    activePanel, setActivePanel,
    activeAdvanced, setActiveAdvanced,
    missingSelection, setMissingSelection,
  };
}

export function useSaveFeedback() {
  const [editorDirty, setEditorDirty] = useState(false);
  const [preview, setPreview] = useState<SavePreview | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [localError, setLocalError] = useState("");
  return {
    editorDirty, setEditorDirty,
    preview, setPreview,
    problem, setProblem,
    localError, setLocalError,
  };
}

export function useOverlays(creating: boolean) {
  const [creatingConnection, setCreatingConnection] = useState(creating);
  const [launching, setLaunching] = useState(false);
  const [managing, setManaging] = useState(false);
  return {
    creatingConnection, setCreatingConnection,
    launching, setLaunching,
    managing, setManaging,
  };
}
