import { useCallback, useEffect, useState } from "react";
import type { FieldEdit, GroupMetadata, HostDetail, SavePreview, UpdateConnectionRequest } from "../api/config";
import type { Problem } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import {
  connectionAreaForTab,
  tabForConnectionArea,
  type AdvancedArea,
  type ConnectionArea,
  type HostEditorTab,
} from "../routing/connectionRoute";
import { AdvancedSettings } from "./AdvancedSettings";
import { ConnectionAnalysis } from "./ConnectionAnalysis";
import { ConnectionBasicForm } from "./ConnectionBasicForm";
import { ConnectionChecks } from "./ConnectionChecks";
import type { ConnectionSavedState } from "./connectionSavedState";
import { NoticeList, SavePreviewPanel } from "./SavePreview";

type HostDetailPanelProps = {
  detail: HostDetail;
  savedState?: ConnectionSavedState | undefined;
  preview: SavePreview | null;
  problem: Problem | null;
  onFieldEdits: (edits: FieldEdit[]) => void;
  onBlockRaw: (raw: string) => void;
  onBasicSave: (request: UpdateConnectionRequest) => Promise<void>;
  integrations?: IntegrationsApi;
  tab?: HostEditorTab;
  onTabChange?: (tab: HostEditorTab) => void;
  preferredKey?: GeneratedPrivateKeyHandoff | null | undefined;
  onPreferredKeyApplied?: (() => void) | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
  onBasicDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
  onRequestRefresh?: (() => Promise<void>) | undefined;
  disabled?: boolean | undefined;
  // Transitional compatibility for the page while management is moved into
  // ManageConnection. HostDetail deliberately does not render these actions.
  groups?: GroupMetadata[] | undefined;
  onRename?: ((alias: string) => void) | undefined;
  onComment?: ((comment: string) => void) | undefined;
  onMoveToGroup?: ((group: string) => void) | undefined;
};

const areas: { area: ConnectionArea; label: "conn.areaBasic" | "conn.areaAnalysis" | "conn.areaAdvanced" }[] = [
  { area: "Basic", label: "conn.areaBasic" },
  { area: "Analysis", label: "conn.areaAnalysis" },
  { area: "Advanced", label: "conn.areaAdvanced" },
];

export function HostDetailPanel({
  detail,
  savedState,
  preview,
  problem,
  onFieldEdits,
  onBlockRaw,
  onBasicSave,
  integrations = integrationsApi,
  tab: controlledTab,
  onTabChange,
  preferredKey,
  onPreferredKeyApplied,
  onDirtyChange,
  onBasicDiscardReady,
  onRequestRefresh,
  disabled = false,
}: HostDetailPanelProps) {
  const t = useTranslate();
  const [localTab, setLocalTab] = useState<HostEditorTab>("Basic");
  const [lastAdvanced, setLastAdvanced] = useState<AdvancedArea>("Jump");
  const [basicDirty, setBasicDirty] = useState(false);
  const [advancedDirty, setAdvancedDirty] = useState(false);
  const tab = controlledTab ?? localTab;
  const route = connectionAreaForTab(tab);
  const advancedArea = route.area === "Advanced" ? route.advanced : lastAdvanced;
  const dirty = basicDirty || advancedDirty;
  const identity = detail.form.entry.identity;
  const resetKey = `${identity.path}\u0000${identity.alias}\u0000${detail.file.contents}`;

  useEffect(() => {
    if (route.area === "Advanced") setLastAdvanced(route.advanced);
  }, [route.advanced, route.area]);

  useEffect(() => {
    if (controlledTab === undefined) setLocalTab("Basic");
    setBasicDirty(false);
    setAdvancedDirty(false);
    // resetKey is the committed snapshot the mounted drafts belong to. A
    // controlled tab change is only a view change and must not clear dirtiness.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  const handleBasicDirty = useCallback((next: boolean) => setBasicDirty(next), []);
  const handleAdvancedDirty = useCallback((next: boolean) => setAdvancedDirty(next), []);

  function selectTab(next: HostEditorTab) {
    if (onTabChange !== undefined) onTabChange(next);
    else setLocalTab(next);
  }

  function selectArea(area: ConnectionArea) {
    selectTab(tabForConnectionArea(area, advancedArea, false));
  }

  function selectAdvanced(area: AdvancedArea) {
    setLastAdvanced(area);
    selectTab(tabForConnectionArea("Advanced", area, false));
  }

  return (
    <section className="flex flex-col gap-4">
      <NoticeList notices={detail.form.notices ?? []} />

      <div role="tablist" aria-label={t("conn.editorLabel")} className="flex gap-1 border-b border-line">
        {areas.map((item) => (
          <button
            key={item.area}
            type="button"
            role="tab"
            aria-selected={route.area === item.area}
            onClick={() => selectArea(item.area)}
            className={`px-3 py-2 text-sm ${route.area === item.area ? "border-b-2 border-ink text-ink" : "text-ink-muted"}`}
          >
            {t(item.label)}
          </button>
        ))}
      </div>

      <div hidden={route.area !== "Basic"} className="flex flex-col gap-5">
        {identity.alias === "" ? (
          <p className="text-sm text-ink-muted">{t("host.noDestination")}</p>
        ) : (
          <ConnectionChecks
            alias={identity.alias}
            api={integrations}
            disabled={disabled || dirty}
            resetKey={resetKey}
          />
        )}
        <ConnectionBasicForm
          detail={detail}
          savedState={savedState}
          problem={problem}
          onSave={onBasicSave}
          secrets={integrations}
          preferredKey={preferredKey}
          onPreferredKeyApplied={onPreferredKeyApplied}
          onDirtyChange={handleBasicDirty}
          onDiscardReady={onBasicDiscardReady}
          onRequestRefresh={onRequestRefresh}
          disabled={disabled || advancedDirty}
        />
      </div>

      <div hidden={route.area !== "Analysis"}>
        <ConnectionAnalysis detail={detail} alias={identity.alias} api={integrations} />
      </div>

      <div hidden={route.area !== "Advanced"}>
        <AdvancedSettings
          detail={detail}
          area={advancedArea}
          onAreaChange={selectAdvanced}
          onFieldEdits={onFieldEdits}
          onBlockRaw={onBlockRaw}
          disabled={disabled || basicDirty}
          onDirtyChange={handleAdvancedDirty}
        />
      </div>

      <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={problem} />
    </section>
  );
}
