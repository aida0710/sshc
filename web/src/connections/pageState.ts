import { useState } from "react";
import type { Problem } from "../api/client";
import type { SavePreview } from "../api/config";
import type { AdvancedArea, ConnectionPanel, ConnectionTarget } from "../routing/connectionRoute";
import type { HostSelection } from "./ConnectionTree";

// 接続の画面が抱える状態を、関心ごとに束ねたものである。
//
// **以前は 19 個の useState が同じスコープに並んでいた。** どれが URL に紐づき、
// どれが保存の結果で、どれが開いているダイアログなのかは、1000 行を読まないと
// 分からなかった。返す名前は画面が使っていたものに合わせてある——束ね直しただけで、
// どの操作が何を触るかは変えていない。

// useSelectionState は、いまどのホストのどの面を見ているか。
//
// **これは URL に対応する。** 画面を開き直しても、リンクを共有しても、同じ場所が
// 出る必要があるので、初期値は解析した location から来る。
export function useSelectionState(target: ConnectionTarget | null, invalid: boolean) {
  const [selection, setSelection] = useState<HostSelection | null>(
    target === null ? null : { path: target.path, alias: target.alias },
  );
  const [invalidLocation, setInvalidLocation] = useState(invalid);
  const [activePanel, setActivePanel] = useState<ConnectionPanel>(target?.panel ?? "Basic");
  const [activeAdvanced, setActiveAdvanced] = useState<AdvancedArea>(target?.advanced ?? "Jump");
  // missingSelection は、URL が名指したホストが設定に無いことを言う。**空の詳細を
  // 出さない** ——名前が消えたのか、まだ読み込んでいないのかは、別のことである。
  const [missingSelection, setMissingSelection] = useState(false);
  return {
    selection, setSelection,
    invalidLocation, setInvalidLocation,
    activePanel, setActivePanel,
    activeAdvanced, setActiveAdvanced,
    missingSelection, setMissingSelection,
  };
}

// useSaveFeedback は、保存しようとした結果である。
//
// **preview は保存の前に出るものである。** 何が書かれるかを見せてから書くので、
// 断られた保存の理由（problem）と、こちらで気づいた誤り（localError）は別に持つ。
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

// useOverlays は、いま開いている面である。作成、接続の起動、鍵の管理。
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
