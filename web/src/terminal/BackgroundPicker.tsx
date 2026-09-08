import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalBackground } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { control } from "../ui/form";
import { Icon } from "../ui/icons";
import { InputDialog } from "../ui/InputDialog";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { useBackgroundImage } from "./backgroundImage";

type BackgroundApi = Pick<IntegrationsApi, "terminalBackgrounds" | "addTerminalBackground" | "setTerminalBackgroundCapacity" | "renameTerminalBackground" | "deleteTerminalBackground">;

type BackgroundPickerProps = {
  value: string;
  onChange: (next: string) => void;
  tint: number | undefined;
  onTintChange: (next: number | undefined) => void;
  unchosen: string;
  api?: BackgroundApi;
};

const MiB = 1 << 20;

export function BackgroundPicker({ value, onChange, tint, onTintChange, unchosen, api = integrationsApi }: BackgroundPickerProps) {
  const t = useTranslate();
  const [stored, setStored] = useState<TerminalBackground[]>([]);
  const [used, setUsed] = useState(0);
  const [capacity, setCapacity] = useState(16 * MiB);
  const [capacityInput, setCapacityInput] = useState("16");
  const [problem, setProblem] = useState("");
  const [renameProblem, setRenameProblem] = useState("");
  const [renameTarget, setRenameTarget] = useState<TerminalBackground | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TerminalBackground | null>(null);
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const chooser = useRef<HTMLInputElement>(null);
  const openButton = useRef<HTMLButtonElement>(null);

  const applyList = useCallback((listed: Awaited<ReturnType<BackgroundApi["terminalBackgrounds"]>>) => {
    setStored(listed.backgrounds);
    setUsed(listed.usedBytes);
    setCapacity(listed.capacityBytes);
    setCapacityInput(String(Math.round(listed.capacityBytes / MiB)));
  }, []);
  const reload = useCallback(async () => applyList(await api.terminalBackgrounds()), [api, applyList]);
  useEffect(() => { void reload().catch(() => undefined); }, [reload]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return needle === "" ? stored : stored.filter((image) => image.name.toLocaleLowerCase().includes(needle));
  }, [query, stored]);

  async function add(file: File) {
    setBusy(true); setProblem("");
    try {
      const added = await api.addTerminalBackground(file.name, file);
      await reload();
      setDraft(added.name);
    } catch (error) {
      const code = failureCode(error);
      setProblem(code === "background_too_large" ? t("terminal.backgroundTooLarge") : code === "backgrounds_full" ? t("terminal.backgroundsFull") : code === "not_an_image" ? t("terminal.backgroundNotAnImage") : t("terminal.backgroundFailed"));
    } finally { setBusy(false); }
  }

  async function saveCapacity() {
    const next = Number(capacityInput);
    if (!Number.isSafeInteger(next) || next < 1 || next > 1024) { setProblem(t("terminal.backgroundCapacityInvalid")); return; }
    setBusy(true); setProblem("");
    try { applyList(await api.setTerminalBackgroundCapacity(next)); }
    catch { setProblem(t("terminal.backgroundCapacityFailed")); }
    finally { setBusy(false); }
  }

  async function drop(name: string) {
    setBusy(true); setProblem("");
    try {
      await api.deleteTerminalBackground(name);
      if (draft === name) setDraft("");
      if (value === name) onChange("");
      setDeleteTarget(null);
      await reload();
    } catch { setProblem(t("terminal.backgroundFailed")); }
    finally { setBusy(false); }
  }

  async function rename(nextName: string) {
    if (renameTarget === null) return;
    setBusy(true); setRenameProblem("");
    try {
      const renamed = await api.renameTerminalBackground(renameTarget.name, nextName);
      if (draft === renameTarget.name) setDraft(renamed.name);
      if (value === renameTarget.name) onChange(renamed.name);
      setRenameTarget(null);
      await reload();
    } catch (error) {
      setRenameProblem(failureCode(error) === "background_already_exists" ? t("terminal.backgroundRenameExists") : t("terminal.backgroundRenameFailed"));
    } finally { setBusy(false); }
  }

  const selected = stored.find((image) => image.name === value);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex min-w-0 flex-wrap items-center gap-3 rounded-md bg-surface-subtle p-2.5 sm:flex-nowrap">
        {selected === undefined ? <div className="flex size-14 shrink-0 items-center justify-center rounded-md bg-control text-ink-faint"><Icon name="config" className="size-5" /></div> : <Thumbnail name={selected.name} chosen className="size-14 rounded-md" />}
        <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium text-ink">{selected?.name ?? unchosen}</p><p className="mt-0.5 text-xs text-ink-faint">{selected === undefined ? t("terminal.backgroundNotSelected") : formatBytes(selected.bytes)}</p></div>
        <Button ref={openButton} onClick={() => { setDraft(value); setProblem(""); setLibraryOpen(true); }}>{t("terminal.backgroundChoose")}</Button>
        {value === "" ? null : <Button onClick={() => onChange("")}>{t("terminal.backgroundClear")}</Button>}
      </div>

      {value === "" ? null : <label className="flex flex-col gap-1"><span className="text-xs text-ink-muted">{t("terminal.tintLabel")} · {tint ?? ""}</span><input type="range" min={0} max={100} step={5} value={tint ?? 55} onChange={(event) => onTintChange(Number(event.target.value))} /><span className="text-xs text-ink-faint">{t("terminal.tintHint")}</span></label>}

      {libraryOpen ? <ModalShell labelledBy="terminal-background-library-heading" onDismiss={() => setLibraryOpen(false)} returnFocusRef={openButton} placement="sheet" panelClassName="flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg">
        <header className="flex items-center gap-3 border-b border-line bg-toolbar px-4 py-3"><h3 id="terminal-background-library-heading" className="min-w-0 flex-1 text-sm font-semibold text-ink">{t("terminal.backgroundLibrary")}</h3><Button onClick={() => setLibraryOpen(false)}>{t("terminal.backgroundClose")}</Button></header>
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
          <div className="flex flex-col gap-2 sm:flex-row"><label className="relative min-w-0 flex-1"><span className="sr-only">{t("terminal.backgroundSearch")}</span><Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-faint" /><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} className={`${control} pl-9`} placeholder={t("terminal.backgroundSearch")} /></label><input ref={chooser} type="file" className="hidden" accept="image/png,image/jpeg,image/webp,image/gif" onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ""; if (file !== undefined) void add(file); }} /><Button kind="primary" onClick={() => chooser.current?.click()} disabled={busy}>{t("terminal.backgroundAdd")}</Button></div>
          {filtered.length === 0 ? <div className="flex min-h-48 flex-col items-center justify-center rounded-md bg-surface-subtle px-4 text-center"><p className="text-sm font-medium text-ink">{t("terminal.backgroundEmpty")}</p><p className="mt-1 text-xs text-ink-muted">{t("terminal.backgroundEmptyHint")}</p></div> : <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">{filtered.map((background) => <li key={background.name} className={`relative overflow-hidden rounded-md bg-surface-subtle ring-1 ${draft === background.name ? "ring-accent" : "ring-hairline"}`}><button type="button" onClick={() => setDraft(background.name)} className="block w-full text-left"><Thumbnail name={background.name} chosen={draft === background.name} className="aspect-video w-full" /><span className="block px-2.5 pb-2.5 pt-2"><span className="block truncate text-sm font-medium text-ink">{background.name}</span><span className="mt-0.5 block text-xs text-ink-faint">{formatBytes(background.bytes)}</span></span></button><details className="absolute right-2 top-2"><summary aria-label={t("terminal.backgroundActions", { name: background.name })} className="flex size-8 cursor-pointer list-none items-center justify-center rounded-md bg-canvas/80 text-ink shadow-sm marker:hidden hover:bg-card"><Icon name="moreHorizontal" className="size-4" /></summary><div className="absolute right-0 top-9 z-10 min-w-36 rounded-md border border-line bg-card p-1 shadow-xl"><button type="button" className="block w-full rounded px-2.5 py-2 text-left text-sm text-ink hover:bg-select-fill" onClick={() => { setRenameProblem(""); setRenameTarget(background); }}>{t("terminal.backgroundRenameAction")}</button><button type="button" className="block w-full rounded px-2.5 py-2 text-left text-sm text-danger hover:bg-select-fill" onClick={() => setDeleteTarget(background)}>{t("terminal.backgroundDeleteAction")}</button></div></details></li>)}</ul>}
          <section className="rounded-md bg-surface-subtle p-3" aria-label={t("terminal.backgroundCapacityHeading")}><div className="flex flex-col gap-3 sm:flex-row sm:items-end"><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-3 text-xs text-ink-muted"><span>{t("terminal.backgroundCapacityUsage", { used: formatBytes(used), capacity: formatBytes(capacity) })}</span><span>{Math.min(100, Math.round((used / Math.max(capacity, 1)) * 100))}%</span></div><div className="mt-2 h-1.5 overflow-hidden rounded-full bg-control"><div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(100, (used / Math.max(capacity, 1)) * 100)}%` }} /></div></div><label className="flex items-end gap-2"><span className="flex flex-col gap-1 text-xs text-ink-muted">{t("terminal.backgroundCapacityLabel")}<span className="flex items-center gap-1"><input type="number" min={1} max={1024} value={capacityInput} onChange={(event) => setCapacityInput(event.target.value)} className={`${control} w-24`} /><span>MiB</span></span></span><Button disabled={busy || capacityInput === String(Math.round(capacity / MiB))} onClick={() => void saveCapacity()}>{t("terminal.backgroundCapacitySave")}</Button></label></div><p className="mt-2 text-xs text-ink-faint">{t("terminal.backgroundCapacityHint")}</p></section>
          {problem === "" ? null : <p role="alert" className="text-xs text-danger">{problem}</p>}
        </div>
        <footer className="flex items-center justify-between gap-3 border-t border-line bg-toolbar px-4 py-3"><button type="button" className="text-sm text-ink-muted hover:text-ink" onClick={() => setDraft("")}>{t("terminal.backgroundUseNone")}</button><Button kind="primary" disabled={busy} onClick={() => { onChange(draft); setLibraryOpen(false); }}>{t("terminal.backgroundUse")}</Button></footer>
      </ModalShell> : null}

      {renameTarget === null ? null : <InputDialog key={renameTarget.name} id="terminal-background-rename" heading={t("terminal.backgroundRenameHeading")} description={renameProblem === "" ? t("terminal.backgroundRenameHint") : <span role="alert" className="text-danger">{renameProblem}</span>} label={t("terminal.backgroundRenameLabel")} initialValue={renameTarget.name} submitLabel={t("terminal.backgroundRenameSubmit")} cancelLabel={t("terminal.backgroundRenameCancel")} validate={(next) => next === "" ? t("terminal.backgroundRenameRequired") : ""} onSubmit={(next) => void rename(next)} onCancel={() => { setRenameProblem(""); setRenameTarget(null); }} />}
      {deleteTarget === null ? null : <ConfirmDialog id="terminal-background-delete" heading={t("terminal.backgroundDeleteHeading")} body={<p className="text-sm text-ink-muted">{t("terminal.backgroundDeleteHint", { name: deleteTarget.name })}</p>} confirmLabel={t("terminal.backgroundDeleteAction")} cancelLabel={t("terminal.backgroundRenameCancel")} onConfirm={() => void drop(deleteTarget.name)} onCancel={() => setDeleteTarget(null)} />}
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GiB`;
  if (bytes >= MiB) return `${(bytes / MiB).toFixed(bytes >= 10 * MiB ? 0 : 1)} MiB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KiB`;
  return `${bytes} B`;
}

function Thumbnail({ name, chosen, className = "h-16 w-24 rounded-md" }: { name: string; chosen: boolean; className?: string }) {
  const url = useBackgroundImage(name);
  if (url === "") return <div className={`${className} bg-control`} />;
  return <img src={url} alt={name} className={`${className} object-cover ${chosen ? "brightness-105" : ""}`} />;
}
