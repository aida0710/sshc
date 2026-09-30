import { useCallback, useEffect, useRef, useState } from "react";
import type { Section } from "../routing/sectionRoute";
import type { InspectorContent } from "../ui/Inspector";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMediaQuery } from "../ui/useMediaQuery";

// Below Tailwind's lg breakpoint the layout has no third column, so the
// inspector opens over the section as a modal layer.
const inspectorOverlayQuery = "(max-width: 1023px)";

const androidBackEvent = "sshc-android-back";

type ShellLayersOptions = {
  mobileLayout: boolean;
  ready: boolean;
  section: Section | null;
};

// useShellLayers は、画面の枠の上に一時的に開く3つの層（ナビゲーション、
// インスペクタ、コマンドパレット）の開閉と、閉じたときにフォーカスを返す先を持つ。
// Android の戻るボタンは、開いている層を上から1つ閉じる。
export function useShellLayers({ mobileLayout, ready, section }: ShellLayersOptions) {
  const [navigationOpen, setNavigationOpen] = useState(false);
  const navigationPanelRef = useRef<HTMLElement>(null);
  const navigationTriggerRef = useRef<HTMLButtonElement>(null);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspector, setInspector] = useState<InspectorContent>(null);
  const inspectorPanelRef = useRef<HTMLElement>(null);
  const inspectorTriggerRef = useRef<HTMLButtonElement>(null);
  const inspectorIsOverlay = useMediaQuery(inspectorOverlayQuery);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const commandPaletteReturnFocusRef = useRef<HTMLElement>(null);

  useDismissibleLayer({
    open: navigationOpen,
    containerRefs: [navigationPanelRef, navigationTriggerRef],
    onDismiss: () => setNavigationOpen(false),
    returnFocusRef: navigationTriggerRef,
    trapFocus: mobileLayout,
  });
  useDismissibleLayer({
    open: inspectorOpen && inspector !== null && inspectorIsOverlay,
    containerRefs: [inspectorPanelRef, inspectorTriggerRef],
    onDismiss: () => setInspectorOpen(false),
    closeOnOutside: false,
    returnFocusRef: inspectorTriggerRef,
    initialFocusRef: inspectorPanelRef,
    trapFocus: true,
  });

  useEffect(() => {
    if (!ready) setCommandPaletteOpen(false);
  }, [ready]);

  // Each section offers its own inspector, if any.
  useEffect(() => {
    setInspector(null);
  }, [section]);

  useEffect(() => {
    function closeTopLayer(event: Event) {
      if (commandPaletteOpen) {
        event.preventDefault();
        setCommandPaletteOpen(false);
      } else if (navigationOpen) {
        event.preventDefault();
        setNavigationOpen(false);
      } else if (inspectorOpen) {
        event.preventDefault();
        setInspectorOpen(false);
      }
    }
    window.addEventListener(androidBackEvent, closeTopLayer);
    return () => window.removeEventListener(androidBackEvent, closeTopLayer);
  }, [commandPaletteOpen, inspectorOpen, navigationOpen]);

  const closeNavigation = useCallback(() => setNavigationOpen(false), []);

  // The palette gives focus back to what had it when it opened.
  const openPalette = useCallback(() => {
    commandPaletteReturnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setCommandPaletteOpen(true);
  }, []);

  // Opened from the navigation, which closes behind it, the palette gives
  // focus back to the button that opened the navigation.
  const openPaletteFromNavigation = useCallback(() => {
    commandPaletteReturnFocusRef.current = navigationTriggerRef.current;
    setNavigationOpen(false);
    setCommandPaletteOpen(true);
  }, []);

  return {
    navigationOpen,
    setNavigationOpen,
    closeNavigation,
    navigationPanelRef,
    navigationTriggerRef,
    inspector,
    setInspector,
    inspectorOpen,
    setInspectorOpen,
    inspectorPanelRef,
    inspectorTriggerRef,
    commandPaletteOpen,
    closePalette: () => setCommandPaletteOpen(false),
    commandPaletteReturnFocusRef,
    openPalette,
    openPaletteFromNavigation,
  };
}
