import { useCallback, useEffect, useState } from "react";
import type { FieldEdit, HostDetail, SavePreview, UpdateConnectionRequest } from "../api/config";
import type { Problem } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import {
  type AdvancedArea,
  type ConnectionPanel,
} from "../routing/connectionRoute";
import { AdvancedSettings } from "./AdvancedSettings";
import { ConnectionAnalysis } from "./ConnectionAnalysis";
import { ConnectionBasicForm } from "./ConnectionBasicForm";
import { ConnectionChecks } from "./ConnectionChecks";
import type { ConnectionSavedState } from "./connectionSavedState";
import { NoticeList, SavePreviewPanel } from "./SavePreview";
import { identityKey } from "./connectionBrowser";

type HostDetailPanelProps = {
  detail: HostDetail;
  savedState: ConnectionSavedState;
  preview: SavePreview | null;
  problem: Problem | null;
  onFieldEdits: (edits: FieldEdit[]) => void;
  onBlockRaw: (raw: string) => void;
  onBasicSave: (request: UpdateConnectionRequest) => Promise<void>;
  integrations?: IntegrationsApi;
  panel?: ConnectionPanel;
  advanced?: AdvancedArea;
  onLocationChange?: (panel: ConnectionPanel, advanced: AdvancedArea) => void;
  preferredKey?: GeneratedPrivateKeyHandoff | null | undefined;
  onPreferredKeyApplied?: (() => void) | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
  onBasicDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
  onRequestRefresh?: (() => Promise<void>) | undefined;
  disabled?: boolean | undefined;
  savedRevision?: number | undefined;
};

const areas: { area: ConnectionPanel; label: "conn.areaBasic" | "conn.areaAnalysis" | "conn.areaAdvanced" }[] = [
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
  panel: controlledPanel,
  advanced: controlledAdvanced,
  onLocationChange,
  preferredKey,
  onPreferredKeyApplied,
  onDirtyChange,
  onBasicDiscardReady,
  onRequestRefresh,
  disabled = false,
  savedRevision = 0,
}: HostDetailPanelProps) {
  const t = useTranslate();
  const [localPanel, setLocalPanel] = useState<ConnectionPanel>("Basic");
  const [lastAdvanced, setLastAdvanced] = useState<AdvancedArea>("Jump");
  const [basicDirty, setBasicDirty] = useState(false);
  const [advancedDirty, setAdvancedDirty] = useState(false);
  const panel = controlledPanel ?? localPanel;
  const advancedArea = panel === "Advanced" ? (controlledAdvanced ?? lastAdvanced) : lastAdvanced;
  const dirty = basicDirty || advancedDirty;
  const identity = detail.form.entry.identity;
  const resetKey = `${identityKey(identity)}\u0000${detail.file.contents}\u0000${savedRevision}`;

  useEffect(() => {
    if (panel === "Advanced") setLastAdvanced(advancedArea);
  }, [advancedArea, panel]);

  useEffect(() => {
    if (controlledPanel === undefined) setLocalPanel("Basic");
    setLastAdvanced("Jump");
    setBasicDirty(false);
    setAdvancedDirty(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  const handleBasicDirty = useCallback((next: boolean) => setBasicDirty(next), []);
  const handleAdvancedDirty = useCallback((next: boolean) => setAdvancedDirty(next), []);

  function selectArea(area: ConnectionPanel) {
    if (onLocationChange !== undefined) onLocationChange(area, advancedArea);
    else setLocalPanel(area);
  }

  function selectAdvanced(area: AdvancedArea) {
    setLastAdvanced(area);
    if (onLocationChange !== undefined) onLocationChange("Advanced", area);
    else setLocalPanel("Advanced");
  }

  return (
    <section className="flex flex-col gap-5">
      <NoticeList notices={detail.form.notices ?? []} />

      <div role="tablist" aria-label={t("conn.editorLabel")} className="sticky top-0 z-10 flex gap-1 rounded-lg bg-select-fill p-1 shadow-sm">
        {areas.map((item) => (
          <button
            key={item.area}
            type="button"
            role="tab"
            aria-selected={panel === item.area}
            onClick={() => selectArea(item.area)}
            className={`flex-1 rounded-md px-3 py-2 text-sm transition-colors ${panel === item.area ? "bg-card font-medium text-ink shadow-sm" : "text-ink-muted hover:text-ink"}`}
          >
            {t(item.label)}
          </button>
        ))}
      </div>

      <div hidden={panel !== "Basic"} className="flex flex-col gap-5">
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

      <div hidden={panel !== "Analysis"}>
        <ConnectionAnalysis detail={detail} alias={identity.alias} api={integrations} disabled={disabled || dirty} />
      </div>

      <div hidden={panel !== "Advanced"}>
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
