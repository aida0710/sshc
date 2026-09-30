import { useEffect, useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import {
  connectionLocation,
  parseConnectionLocation,
  type AdvancedArea,
  type ConnectionPanel,
} from "../routing/connectionRoute";
import type { BrowserLocation, NavigateLocationOptions } from "../routing/useSectionRoute";
import type { HostSelection } from "./ConnectionTree";

type ConnectionSelectionOptions = {
  location: BrowserLocation;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => boolean | void) | undefined;
  // 別の接続へ選択を移した（選択を外した）とき。読み込んだ内容、保存の結果と失敗の文、読み直しの状態、
  // 未保存の印、管理のパネルは、どれも選んでいた接続についてのものなので、呼び出し側で全部戻す。
  onSelectionMoved: () => void;
  // 保存で同じ接続の識別子が変わったとき。読み込んだ内容だけを捨て、直前の保存の結果（preview）は見せ続ける。
  onIdentityFollowed: () => void;
};

// useConnectionSelection は、Connections画面のURLと、選んでいる接続・開いているタブを同期する。
// URLが選択の唯一の出どころで、選択を変える操作はURLを書き換えてから選択を移す。
export function useConnectionSelection({
  location,
  onNavigateLocation,
  onSelectionMoved,
  onIdentityFollowed,
}: ConnectionSelectionOptions) {
  const [initialRoute] = useState(() => parseConnectionLocation(location));
  const initialTarget = initialRoute.kind === "valid" ? initialRoute.target : null;
  const [selection, setSelection] = useState<HostSelection | null>(
    initialTarget === null ? null : { path: initialTarget.path, alias: initialTarget.alias },
  );
  const [invalidLocation, setInvalidLocation] = useState(initialRoute.kind === "invalid");
  const [activePanel, setActivePanel] = useState<ConnectionPanel>(initialTarget?.panel ?? "Basic");
  const [activeAdvanced, setActiveAdvanced] = useState<AdvancedArea>(initialTarget?.advanced ?? "Jump");
  // 非同期の処理が終わったときに、まだ同じ接続を選んでいるかを確かめるために、最新の選択を持つ。
  const selectionRef = useRef<HostSelection | null>(selection);

  useEffect(() => {
    selectionRef.current = selection;
  }, [selection]);

  function isCurrentSelection(identity: HostSelection): boolean {
    const current = selectionRef.current;
    return current?.path === identity.path && current.alias === identity.alias;
  }

  function emitLocation(url: string, options?: NavigateLocationOptions): boolean {
    const result = options === undefined
      ? onNavigateLocation?.(url)
      : onNavigateLocation?.(url, options);
    return result !== false;
  }

  function navigateTarget(
    identity: HostSelection,
    panel: ConnectionPanel,
    advanced: AdvancedArea,
    options?: NavigateLocationOptions,
  ): boolean {
    if (!emitLocation(connectionLocation({
      path: identity.path,
      alias: identity.alias,
      panel,
      advanced,
    }), options)) return false;
    setActivePanel(panel);
    setActiveAdvanced(advanced);
    setInvalidLocation(false);
    return true;
  }

  function clearTarget(options?: NavigateLocationOptions): boolean {
    return emitLocation(connectionLocation(null), options);
  }

  function dismissInvalidLocation() {
    if (clearTarget({ replace: true })) setInvalidLocation(false);
  }

  function followCommittedIdentity(
    identity: HostSelection,
    panel: ConnectionPanel = activePanel,
    advanced: AdvancedArea = activeAdvanced,
  ) {
    selectionRef.current = identity;
    setSelection(identity);
    onIdentityFollowed();
    // URLを書き換えられなかったときも、開くタブは呼び出し側が指定したものにする。
    setActivePanel(panel);
    setActiveAdvanced(advanced);
    navigateTarget(identity, panel, advanced, { replace: true });
  }

  function leaveCommittedIdentityUnknown() {
    selectionRef.current = null;
    setSelection(null);
    onIdentityFollowed();
    clearTarget({ replace: true });
  }

  function moveSelectionTo(next: HostSelection | null) {
    selectionRef.current = next;
    setSelection(next);
    onSelectionMoved();
  }

  function clearSelection() {
    if (selectionRef.current === null) return;
    moveSelectionTo(null);
    setActivePanel("Basic");
    setActiveAdvanced("Jump");
  }

  function selectHost(host: HostEntry) {
    if (host.identity.alias === "") return;
    const nextSelection = { path: host.identity.path, alias: host.identity.alias };
    const selectingCurrent = isCurrentSelection(nextSelection);
    if (!navigateTarget(nextSelection, "Basic", "Jump")) return;
    if (selectingCurrent) return;
    moveSelectionTo(nextSelection);
  }

  useEffect(() => {
    const parsed = parseConnectionLocation(location);
    if (parsed.kind === "redirect") {
      emitLocation(parsed.location, { replace: true });
      setInvalidLocation(false);
      clearSelection();
      return;
    }
    if (parsed.kind === "invalid") {
      setInvalidLocation(true);
      clearSelection();
      return;
    }

    setInvalidLocation(false);
    const target = parsed.target;
    if (target === null) {
      clearSelection();
      return;
    }

    setActivePanel(target.panel);
    setActiveAdvanced(target.advanced);
    if (isCurrentSelection(target)) return;
    moveSelectionTo({ path: target.path, alias: target.alias });
    // The URL is the single source for the selection; the state setters and
    // the current selection ref are read, not watched, so navigating back to
    // the same location does not reset an open editor.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname, location.search]);

  return {
    selection,
    invalidLocation,
    activePanel,
    activeAdvanced,
    isCurrentSelection,
    navigateTarget,
    clearTarget,
    dismissInvalidLocation,
    followCommittedIdentity,
    leaveCommittedIdentityUnknown,
    clearSelection,
    selectHost,
  };
}
