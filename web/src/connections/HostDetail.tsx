import { useCallback, useEffect, useState } from "react";
import type { FieldEdit, HostDetail, HostMetadata, SavePreview, UpdateConnectionRequest } from "../api/config";
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
import { HostInspector } from "./HostInspector";

type HostDetailPanelProps = {
  detail: HostDetail;
  savedState: ConnectionSavedState;
  preview: SavePreview | null;
  problem: Problem | null;
  onFieldEdits: (edits: FieldEdit[]) => void;
  onBlockRaw: (raw: string) => void;
  onBasicSave: (request: UpdateConnectionRequest) => Promise<void>;
  onMetadata: (metadata: HostMetadata) => void;
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

const areas: { area: ConnectionPanel; label: "conn.areaBasic" | "conn.areaAnalysis" | "conn.areaAdvanced" | "conn.areaSshc" }[] = [
  { area: "Basic", label: "conn.areaBasic" },
  { area: "Analysis", label: "conn.areaAnalysis" },
  { area: "Advanced", label: "conn.areaAdvanced" },
  { area: "Sshc", label: "conn.areaSshc" },
];

export function HostDetailPanel({
  detail,
  savedState,
  preview,
  problem,
  onFieldEdits,
  onBlockRaw,
  onBasicSave,
  onMetadata,
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

      {panel === "Basic" && identity.alias !== "" ? (
        <ConnectionChecks
          alias={identity.alias}
          api={integrations}
          disabled={disabled || dirty}
          resetKey={resetKey}
        />
      ) : null}

      <div data-connection-editor className="min-w-0">
        <div role="tablist" aria-label={t("conn.editorLabel")} className="grid grid-cols-4 border-b border-line">
          {areas.map((item) => (
            <button
              key={item.area}
              id={`connection-area-${item.area.toLowerCase()}-tab`}
              type="button"
              role="tab"
              aria-selected={panel === item.area}
              aria-controls={`connection-area-${item.area.toLowerCase()}-panel`}
              onClick={() => selectArea(item.area)}
              className={`min-h-11 whitespace-nowrap border-b-2 px-2 py-2.5 text-sm transition-colors ${panel === item.area ? "border-accent font-medium text-ink" : "border-transparent text-ink-muted hover:bg-select-fill/50 hover:text-ink"}`}
            >
              {t(item.label)}
            </button>
          ))}
        </div>

        <div className="pt-5">
          <div
            id="connection-area-basic-panel"
            role="tabpanel"
            aria-labelledby="connection-area-basic-tab"
            hidden={panel !== "Basic"}
            className="flex flex-col gap-5"
          >
            {identity.alias === "" ? <p className="text-sm text-ink-muted">{t("host.noDestination")}</p> : null}
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

          <div
            id="connection-area-analysis-panel"
            role="tabpanel"
            aria-labelledby="connection-area-analysis-tab"
            hidden={panel !== "Analysis"}
          >
            <ConnectionAnalysis detail={detail} alias={identity.alias} api={integrations} disabled={disabled || dirty} />
          </div>

          <div
            id="connection-area-advanced-panel"
            role="tabpanel"
            aria-labelledby="connection-area-advanced-tab"
            hidden={panel !== "Advanced"}
          >
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

          <div
            id="connection-area-sshc-panel"
            role="tabpanel"
            aria-labelledby="connection-area-sshc-tab"
            hidden={panel !== "Sshc"}
          >
            <HostInspector detail={detail} onMetadata={onMetadata} />
          </div>
        </div>
      </div>

      <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={problem} />
    </section>
  );
}
