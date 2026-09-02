import { useCallback, useEffect, useRef, useState } from "react";
import { toProblem } from "../api/guards";
import type { Problem } from "../api/client";
import { configApi, type GroupMetadata, type Metadata, type Overview, type SavePreview } from "../api/config";
import { NoticeList, SavePreviewPanel } from "../connections/SavePreview";
import { formatValues, parseValues } from "../rules/rules";
import {
  Field,
  control,
  fieldLabel,
  hintText,
  narrowControl,
  sectionHeading,
} from "../ui/form";
import { useTranslate } from "../i18n/context";
import { Button, Card, Notice } from "../ui/surface";
import type { InspectorContent } from "../ui/Inspector";
import { GroupInspector } from "./GroupInspector";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";


export function depthOf(name: string): number {
  return name.split("/").length;
}

export function treeOrder(groups: GroupMetadata[]): GroupMetadata[] {
  const orderOf = new Map(groups.map((group) => [group.name, group.order ?? 0]));
  const keyOf = (name: string): (number | string)[] => {
    const key: (number | string)[] = [];
    let prefix = "";
    for (const segment of name.split("/")) {
      prefix = prefix === "" ? segment : `${prefix}/${segment}`;
      key.push(orderOf.get(prefix) ?? 0, segment);
    }
    return key;
  };
  return [...groups].sort((left, right) => {
    const leftKey = keyOf(left.name);
    const rightKey = keyOf(right.name);
    for (let index = 0; index < Math.min(leftKey.length, rightKey.length); index += 1) {
      const first = leftKey[index]!;
      const second = rightKey[index]!;
      if (first === second) continue;
      return typeof first === "number" && typeof second === "number"
        ? first - second
        : String(first).localeCompare(String(second));
    }
    return leftKey.length - rightKey.length;
  });
}

export { isValidGroupName } from "../rules/rules";
import { isValidGroupName } from "../rules/rules";

type GroupsPanelProps = {
  onInspector?: (content: InspectorContent) => void;
};

export function GroupsPanel({ onInspector }: GroupsPanelProps = {}) {
  const t = useTranslate();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [metadata, setMetadata] = useState<Metadata | null>(null);
  const [preview, setPreview] = useState<SavePreview | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [newName, setNewName] = useState("");
  const [settingKeyword, setSettingKeyword] = useState("");
  const [settingValue, setSettingValue] = useState("");
  const [renaming, setRenaming] = useState<Record<string, string>>({});
  const [removing, setRemoving] = useState<Record<string, string>>({});
  const [confirmingRemove, setConfirmingRemove] = useState<Record<string, boolean>>({});
  const [localError, setLocalError] = useState("");
  const [selected, setSelected] = useState("");
  const newNameInput = useRef<HTMLInputElement>(null);

  const reload = useCallback(async () => {
    try {
      const loaded = await configApi.overview();
      setOverview(loaded);
      setMetadata(loaded.metadata);
    } catch (error) {
      setProblem(toProblem(error));
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (onInspector === undefined) return;
    const group = (metadata?.groups ?? []).find((candidate) => candidate.name === selected);
    if (group === undefined) {
      onInspector(null);
      return;
    }
    onInspector({
      label: t("inspector.groupLabel"),
      attention: (overview?.notices ?? []).some(
        (notice) =>
          notice.detail === group.name &&
          ["group_not_declared", "group_directory_missing"].includes(notice.code),
      ),
      body: (
        <GroupInspector
          group={group}
          members={(overview?.hosts ?? [])
            .filter((host) => host.group === group.name)
            .map((host) => host.identity.alias)}
          onUpdate={(patch) => updateGroup(group.name, patch)}
        />
      ),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, metadata, overview, onInspector]);

  if (overview === null || metadata === null) {
    return problem === null ? (
      <PanelState tone="loading" title={t("groups.loading")} />
    ) : (
      <PanelState tone="failed" title={problem.message} {...(problem.detail === undefined ? {} : { detail: problem.detail })} action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>} />
    );
  }

  const loaded: Metadata = metadata;
  const savedMetadata: Metadata = overview.metadata;
  const hosts = overview.hosts;
  const groups = treeOrder(loaded.groups ?? []);

  const groupNotices = (overview.notices ?? []).filter((notice) =>
    ["group_not_declared", "group_directory_missing"].includes(notice.code),
  );
  const savedGroups = new Set((overview.metadata.groups ?? []).map((group) => group.name));
  const unsaved = JSON.stringify(loaded) !== JSON.stringify(overview.metadata);

  function membersOf(name: string): string[] {
    return hosts.filter((host) => host.group === name).map((host) => host.identity.alias);
  }

  function addGroup() {
    if (!isValidGroupName(newName)) {
      setLocalError(t("groups.invalidName"));
      return;
    }
    if (groups.some((group) => group.name.toLowerCase() === newName.toLowerCase())) {
      setLocalError(t("groups.nameTaken"));
      return;
    }
    const added: GroupMetadata = { name: newName };
    setMetadata({ ...loaded, groups: [...groups, added] });
    setNewName("");
    setLocalError("");
  }

  function addSetting() {
    if (selected === "" || settingKeyword === "") {
      setLocalError(t("groups.chooseGroupAndKeyword"));
      return;
    }
    let values: string[];
    try {
      values = parseValues(settingValue);
    } catch {
      setLocalError(t("groups.unbalancedQuote"));
      return;
    }
    setMetadata({
      ...loaded,
      groups: groups.map((group) =>
        group.name === selected
          ? { ...group, settings: [...(group.settings ?? []), { keyword: settingKeyword, values }] }
          : group,
      ),
    });
    setSettingKeyword("");
    setSettingValue("");
    setLocalError("");
  }

  function updateGroup(name: string, change: Partial<GroupMetadata>) {
    setMetadata({
      ...loaded,
      groups: (loaded.groups ?? []).map((group) => (group.name === name ? { ...group, ...change } : group)),
    });
  }

  async function renameGroup(from: string) {
    if (unsaved) {
      setLocalError(t("groups.saveDraftFirst"));
      return;
    }
    const target = (renaming[from] ?? "").trim();
    if (target === "" || target === from) {
      setLocalError(t("groups.renameNeedsName"));
      return;
    }
    if (!isValidGroupName(target)) {
      setLocalError(t("groups.invalidName"));
      return;
    }
    if (groups.some((group) => group.name === target)) {
      setLocalError(t("groups.renameCollides", { name: target }));
      return;
    }
    try {
      const result = await configApi.renameGroup(from, target);
      setPreview(result.preview);
      setProblem(null);
      setLocalError("");
      setRenaming({ ...renaming, [from]: "" });
      await reload();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  async function removeGroup(name: string) {
    if (unsaved) {
      setLocalError(t("groups.saveDraftFirst"));
      return;
    }
    try {
      const result = await configApi.deleteGroup(name, removing[name] ?? "");
      setConfirmingRemove({ ...confirmingRemove, [name]: false });
      setPreview(result.preview);
      setProblem(null);
      setLocalError("");
      await reload();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  async function run(action: "preview" | "save") {
    try {
      if (action === "preview") {
        setPreview(await configApi.preview({ kind: "groups", metadata: loaded }));
        setProblem(null);
        return;
      }
      const result = await configApi.save({ kind: "groups", metadata: loaded });
      setPreview(result.preview);
      setProblem(null);
      await reload();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  function discardDraft() {
    setMetadata(savedMetadata);
    setPreview(null);
    setProblem(null);
    setLocalError("");
    setRenaming({});
    setRemoving({});
    setConfirmingRemove({});
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader title={t("groups.pageTitle")} description={t("groups.pageDescription")} />
      <MetricGrid className="sm:grid-cols-3 lg:grid-cols-3">
        {([
          [t("groups.metricGroups"), groups.length],
          [t("groups.metricConnections"), hosts.filter((host) => host.identity.alias !== "").length],
          [t("groups.metricDraft"), unsaved ? 1 : 0],
        ] as const).map(([label, value], index) => (
          <MetricCard key={String(label)} label={String(label)} value={value} compact attention={unsaved && index === 2} />
        ))}
      </MetricGrid>
      <details className="rounded-lg border border-line bg-surface-subtle px-4 py-3">
        <summary className="cursor-pointer text-sm font-medium text-ink">{t("groups.howItWorks")}</summary>
        <div className="mt-3 border-t border-line pt-3">
          <p className="text-sm text-ink-muted">
            {t("groups.directoryNote", { connections: "connections", keys: "keys" })}
          </p>
          <p className="mt-1 text-xs text-ink-muted">
            {t("groups.compileNote", { file: loaded.groupsFile ?? "groups.sshc.conf" })}
          </p>
          <p className="mt-1 text-xs text-ink-faint">{t("groups.orderNote")}</p>
        </div>
      </details>
      {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}

      <NoticeList notices={groupNotices} />

      <Card as="section" radius="md">
        <div className="border-b border-line bg-surface-subtle px-4 py-3">
          <h3 className={sectionHeading}>{t("groups.addHeading")}</h3>
          <div className="mt-3 flex flex-wrap items-end gap-2">
            <div className="min-w-52 flex-1">
              <Field label={t("groups.newName")} hint={t("groups.nestingNote")}>
                <input
                  ref={newNameInput}
                  id="group-name"
                  value={newName}
                  onChange={(event) => setNewName(event.target.value)}
                  placeholder="work/eu"
                  className={control}
                />
              </Field>
            </div>
            <Button onClick={addGroup} disabled={newName === ""}>
              {t("groups.add")}
            </Button>
          </div>
        </div>

      {groups.length === 0 ? (
        <p className="p-5 text-sm text-ink-muted">{t("groups.empty")}</p>
      ) : null}
      <ul aria-label={t("groups.listLabel")} className="divide-y divide-line">
        {groups.map((group) => (
          <li
            key={group.name}
            onFocus={() => setSelected(group.name)}
            onClick={() => setSelected(group.name)}
            className={`relative px-4 py-3 transition-colors ${
              selected === group.name ? "bg-select-fill" : "hover:bg-surface-subtle"
            }`}
            style={{ marginInlineStart: `${(depthOf(group.name) - 1) * 1.5}rem` }}
          >

            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <h3 className="flex items-baseline gap-2 text-sm font-medium">
                <span
                  aria-hidden="true"
                  className="h-2.5 w-2.5 shrink-0 self-center rounded-full ring-4 ring-surface"
                  style={{ backgroundColor: group.colour === undefined || group.colour === "" ? "var(--ui-ink-faint)" : group.colour }}
                />
                {depthOf(group.name) === 1 ? null : (
                  <span className="text-ink-faint">{group.name.slice(0, group.name.lastIndexOf("/") + 1)}</span>
                )}
                <span>{group.name.slice(group.name.lastIndexOf("/") + 1)}</span>
              </h3>

              {savedGroups.has(group.name) ? null : (
                <span className="rounded border border-notice-line px-1.5 py-0.5 text-[10px] font-normal text-notice-ink">
                  {t("groups.unsaved")}
                </span>
              )}
              <p className="font-mono text-xs text-ink-faint">
                {t("groups.directories", {
                  connections: `connections/${group.name}`,
                  keys: `keys/${group.name}`,
                })}
              </p>

            </div>
            <p className="mt-1 pl-[1.125rem] text-xs text-ink-muted">
              {t("groups.members")}{" "}
              <span>{membersOf(group.name).length === 0 ? t("groups.noMembers") : membersOf(group.name).join(", ")}</span>
            </p>

            {(group.settings ?? []).length === 0 ? null : (
              <ul className="mt-2 ml-[1.125rem] flex flex-wrap gap-1.5 font-mono text-xs text-ink-muted">
                {(group.settings ?? []).map((setting, index) => (
                  <li key={`${setting.keyword}-${index}`} className="rounded-md bg-surface px-2 py-1">{`${setting.keyword} ${formatValues(setting.values)}`}</li>
                ))}
              </ul>
            )}


            {selected !== group.name ? null : (
            <div className="mt-3 flex flex-wrap items-end gap-x-4 gap-y-3 border-t border-line pt-3">
              <p className={`w-full ${hintText}`}>{t("groups.immediateActions")}</p>

              <label htmlFor={`group-rename-${group.name}`} className="flex flex-col gap-1">
                <span className={fieldLabel}>{t("groups.renameShort")}</span>
                <span className="flex items-end gap-2">
                  <input
                    id={`group-rename-${group.name}`}
                    aria-label={t("groups.renameTo", { name: group.name })}
                    value={renaming[group.name] ?? ""}
                    disabled={unsaved}
                    onChange={(event) => setRenaming({ ...renaming, [group.name]: event.target.value })}
                    className={narrowControl}
                  />
                  <Button
                    onClick={() => void renameGroup(group.name)}
                    disabled={!savedGroups.has(group.name) || unsaved}
                  >
                    {t("groups.rename", { name: group.name })}
                  </Button>
                </span>
              </label>

              {confirmingRemove[group.name] !== true ? (
                <Button
                  onClick={() => setConfirmingRemove({ ...confirmingRemove, [group.name]: true })}
                  disabled={!savedGroups.has(group.name) || unsaved}
                >
                  {t("groups.remove", { name: group.name })}
                </Button>
              ) : null}

              <Button
                onClick={() => {
                  setNewName(`${group.name}/`);
                  newNameInput.current?.focus();
                }}
              >
                {t("groups.addChild", { name: group.name })}
              </Button>
            </div>
            )}
            {savedGroups.has(group.name) ? null : (
              <p className="mt-2 text-xs text-notice-ink">{t("groups.newGroupNote")}</p>
            )}

            {confirmingRemove[group.name] !== true ? null : (
              <div
                role="group"
                aria-label={t("groups.removeInto", { name: group.name })}
                className="mt-3 flex flex-col gap-2 rounded border border-control-line bg-card/30 p-3"
              >
                <p className="text-sm text-ink">
                  {membersOf(group.name).length === 0
                    ? t("groups.removeExplainEmpty", { name: group.name })
                    : t("groups.removeExplain", { name: group.name, count: membersOf(group.name).length })}
                </p>
                {membersOf(group.name).length === 0 ? null : (
                  <label htmlFor={`group-move-${group.name}`} className="flex flex-col gap-1">
                    <span className={fieldLabel}>{t("groups.removeIntoShort")}</span>
                    <select
                      id={`group-move-${group.name}`}
                      value={removing[group.name] ?? ""}
                      disabled={unsaved}
                      onChange={(event) => setRemoving({ ...removing, [group.name]: event.target.value })}
                      className={`${control} w-56`}
                    >
                      <option value="">{t("groups.removeIntoNone")}</option>
                      {groups
                        .filter(
                          (candidate) =>
                            candidate.name !== group.name && !candidate.name.startsWith(`${group.name}/`),
                        )
                        .map((candidate) => (
                          <option key={candidate.name} value={candidate.name}>
                            {candidate.name}
                          </option>
                        ))}
                    </select>
                  </label>
                )}

                <p className={hintText}>{t("groups.removeKeepsFiles")}</p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    kind="danger"
                    onClick={() => void removeGroup(group.name)}
                    disabled={unsaved}
                  >
                    {t("groups.removeConfirm", { name: group.name })}
                  </Button>
                  <Button
                    onClick={() => setConfirmingRemove({ ...confirmingRemove, [group.name]: false })}
                  >
                    {t("groups.removeCancel")}
                  </Button>
                </div>
              </div>
            )}
          </li>
        ))}
      </ul>
      </Card>


      {selected === "" ? null : (
      <Card as="section" radius="md" className="flex flex-col gap-4 p-4">
        <h3 className={sectionHeading}>{t("groups.settingHeadingFor", { name: selected })}</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("groups.directive")}>
            <input
              id="setting-keyword"
              value={settingKeyword}
              onChange={(event) => setSettingKeyword(event.target.value)}
              placeholder="ServerAliveInterval"
              className={control}
            />
          </Field>
          <Field label={t("groups.value")}>
            <input
              id="setting-value"
              value={settingValue}
              onChange={(event) => setSettingValue(event.target.value)}
              placeholder="30"
              className={control}
            />
          </Field>
        </div>
        <Button onClick={addSetting} className="self-start">
          {t("groups.addSetting")}
        </Button>
      </Card>
      )}


      {unsaved ? (
        <section
          aria-label={t("groups.unsavedBarLabel")}
          className="flex flex-wrap items-center gap-3 rounded-md border border-notice-line bg-notice p-3"
        >
          <p className="min-w-0 grow text-sm text-notice-ink">{t("groups.unsavedBarNote")}</p>
          <Button onClick={() => void run("preview")}>
            {t("groups.previewChanges")}
          </Button>
          <Button onClick={discardDraft}>
            {t("groups.discard")}
          </Button>
          <Button kind="primary" onClick={() => void run("save")}>
            {t("groups.save")}
          </Button>
        </section>
      ) : (
        <p className="text-xs text-ink-muted">{t("groups.savedNote")}</p>
      )}

      <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={problem} />
    </div>
  );
}
