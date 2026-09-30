import {
  configApi,
  hostMetadataEditRequest,
  type EditRequest,
  type FieldEdit,
  type HostMetadata,
  type Overview,
  type SaveResult,
  type UpdateConnectionRequest,
} from "../api/config";
import { toProblem } from "../api/guards";
import { useTranslate } from "../i18n/context";
import type { HostSelection } from "./ConnectionTree";
import type { SaveFeedback } from "./pageState";
import type { SelectedConnection } from "./useSelectedConnection";

export type SaveAttempt =
  | { saved: false; overview: null }
  | { saved: true; overview: Overview | null };

type ConnectionSaveOptions = {
  selection: HostSelection | null;
  selected: Pick<SelectedConnection, "detail" | "reloadAfterSave" | "refreshCommittedConnection">;
  feedback: Pick<SaveFeedback, "setPreview" | "setProblem" | "setLocalError">;
  reload: () => Promise<Overview | null>;
  followCommittedIdentity: (identity: HostSelection) => void;
};

// useConnectionSave は、接続の設定ファイルへの書き込みと、その結果（preview と失敗）の表示を持つ。
// submit は管理の操作（名前の変更や移動）にも使う。
export function useConnectionSave({
  selection,
  selected,
  feedback,
  reload,
  followCommittedIdentity,
}: ConnectionSaveOptions) {
  const t = useTranslate();
  const { detail } = selected;
  const { setPreview, setProblem, setLocalError } = feedback;

  async function submit(request: EditRequest, reselect = true): Promise<SaveAttempt> {
    let result: Awaited<ReturnType<typeof configApi.save>>;
    try {
      result = await configApi.save(request);
    } catch (error) {
      setPreview(null);
      const rejected = toProblem(error);
      if (request.kind === "duplicate" && rejected.code === "alias_already_declared") {
        setProblem(null);
        setLocalError(t("conn.duplicateAliasTaken", { alias: request.newAlias ?? "" }));
      } else {
        setProblem(rejected);
      }
      return { saved: false, overview: null };
    }

    setPreview(result.preview);
    setProblem(null);
    const selectedBeforeSave = selection;
    const renamedSelection =
      request.kind === "rename" && selectedBeforeSave !== null
        ? {
            path: selectedBeforeSave.path,
            alias: request.newAlias ?? selectedBeforeSave.alias,
          }
        : null;
    if (renamedSelection !== null) followCommittedIdentity(renamedSelection);

    const nextOverview = await reload();
    if (reselect && selectedBeforeSave !== null && renamedSelection === null) {
      await selected.reloadAfterSave(selectedBeforeSave);
    }
    return { saved: true, overview: nextOverview };
  }

  // saveEditorDraft は、接続エディタのタブ（Basic、Advanced、sshc）の下書きを書き込み、
  // 保存済みの接続を読み直す。書き込めた時点で resolve し、読み直しは待たない。タブは
  // useDraftSave で、書き込めた下書きを読み直しが終わるまで表示したまま、未保存の変更には
  // 数えない。そのため、読み直しの途中で別の接続を選んでも、破棄の確認は出ない。読み直すあいだは
  // refreshState がエディタを止めるので、読み直した値で下書きをやり直しても、途中の編集は消えない。
  // 書き込めなかったときは理由を problem に出して reject し、タブは下書きを残す。
  async function saveEditorDraft(write: () => Promise<SaveResult>) {
    let result: SaveResult;
    try {
      result = await write();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      throw error;
    }

    setPreview(result.preview);
    setProblem(null);
    setLocalError("");
    void selected.refreshCommittedConnection();
  }

  async function onBasicSave(request: UpdateConnectionRequest) {
    await saveEditorDraft(() => configApi.updateConnection(request));
  }

  async function onFieldEdits(fields: FieldEdit[]) {
    if (detail === null || selection === null) throw new Error("no_connection_open");
    await saveEditorDraft(() => configApi.save({
      kind: "host_fields",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      fields,
    }));
  }

  async function onBlockRaw(raw: string) {
    if (detail === null || selection === null) throw new Error("no_connection_open");
    await saveEditorDraft(() => configApi.save({ kind: "block_raw", path: selection.path, alias: selection.alias, base: detail.file.contents, raw }));
  }

  // onMetadataSave は、sshcタブの下書きをこの接続 1 件の metadata として保存する。
  // 読み込んだときの値を base に送るので、ほかで変わっていればサーバーが断る。
  async function onMetadataSave(host: HostMetadata) {
    if (detail === null) throw new Error("detail_not_loaded");
    await saveEditorDraft(() => configApi.save(hostMetadataEditRequest(detail.metadata, host)));
  }

  return { submit, onBasicSave, onFieldEdits, onBlockRaw, onMetadataSave };
}
