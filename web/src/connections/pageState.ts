import { useState } from "react";
import type { Problem } from "../api/client";
import type { SavePreview } from "../api/config";
import type { ConnectionAction } from "./ConnectionActionPanel";

// RefreshState は、保存したあとに保存済みの接続を読み直す状態である。refreshing と failed の
// あいだは、画面に出ている保存済みの値が古いかもしれないので、接続と編集を止める。
export type RefreshState = "idle" | "refreshing" | "failed";

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

export type SaveFeedback = ReturnType<typeof useSaveFeedback>;

export function useOverlays(creating: boolean) {
  const [creatingConnection, setCreatingConnection] = useState(creating);
  const [launching, setLaunching] = useState(false);
  // managing は、見出しの「…」から開いている操作の欄（無ければ null）。
  const [managing, setManaging] = useState<ConnectionAction | null>(null);
  return {
    creatingConnection, setCreatingConnection,
    launching, setLaunching,
    managing, setManaging,
  };
}
