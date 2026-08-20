import { useCallback, useEffect, useState, type DragEvent } from "react";
import {
  useAgentForm,
  useGenerationForm,
  useOrganiser,
  usePassphraseForm,
  useRelocateForm,
  useStoredPassphraseForm,
  useStoredPhrases,
} from "./forms";
import { RevealDialog } from "./RevealDialog";
import { KeyTable, type KeyRowActions } from "./KeyTable";
import {
  AgentForm,
  PassphraseForm,
  RelocateForm,
  RelocateResult,
  StoredPassphrasePanel,
  TrashConfirmation,
} from "./KeyForms";
import { rowAction, rowDanger } from "./labels";
import { CopyButton } from "../ui/CopyButton";
import { useTranslate } from "../i18n/context";
import {
  CheckboxField,
  control,
  hintText,
  secondaryAction,
  sectionCard,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { Button, Card, Row } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import {
  keysApi,
  type KeyInventoryResponse,
  type KeyItem,
  type KeysApi,
  type KeyVariant,
  type RelocateKeyResponse,
  type TrashListResponse,
} from "./api";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "./workflow";
import { FolderPane } from "./FolderPane";
import type { InspectorContent } from "../ui/Inspector";
import { KeyInspector } from "./KeyInspector";
import {
  folderRows,
  groupOfKeyPath,
  itemsInFolder,
  moveInto,
  shownItems,
  type ListFilter,
  type MoveTarget,
} from "./organizer";

// groups は宣言済みのグループ名で、シェルが overview から渡す。
// Keys 画面はそれらを推測しない: ディレクトリがグループなのは
// ~/.ssh/config の行がそう言っているからで、その行を読むのは設定エンジンだけだ。
//
// secrets は vault であり、最初の面にメソッドを数個足したものではなく
// 第二の面である: 鍵とは何かはこのパッケージに属し、パスフレーズが
// どこにあるかは vault に属する。これはサーバーが鍵サービスと
// シークレットサービスの間で保つのと同じ分離だ。
type KeysScreenProps = {
  api?: KeysApi;
  onInspector?: (content: InspectorContent) => void;
  groups?: string[];
  secrets?: IntegrationsApi;
  onAssignGeneratedKey?: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey?: (key: GeneratedPublicKeyHandoff) => void;
};

type ScreenState = "loading" | "ready" | "error";

// namedStoredFor と dedicatedStoredFor は、この鍵が既に指している保存値の
// 種類だけを返す。vault は名前・用途・鍵専用 subject には答えるが、値には
// 決して答えない。それでもフィールドを空欄のままにしてよいとは判断できる。





// relocateStem は、ユーザーが置き換えるよう求められている名前の部分だ。
// サーバーの規則を映しているので、フィールドはサーバーが拒否するであろう
// 名前ではなく、実際に relocation が変える名前から始まる。
function relocateStem(item: KeyItem): string {
  const base = item.relativePath.split("/").pop() ?? item.relativePath;
  if (item.kind === "private_key") return base;
  for (const suffix of ["-cert.pub", ".pub"]) {
    if (base.endsWith(suffix) && base.length > suffix.length) return base.slice(0, -suffix.length);
  }
  return base;
}


export function KeysScreen({
  onInspector,
  api = keysApi,
  groups = [],
  secrets = integrationsApi,
  onAssignGeneratedKey,
  onInstallGeneratedKey,
}: KeysScreenProps) {
  const t = useTranslate();
  const [state, setState] = useState<ScreenState>("loading");
  const [inventory, setInventory] = useState<KeyInventoryResponse | null>(null);
  const [trash, setTrash] = useState<TrashListResponse | null>(null);
  const [variants, setVariants] = useState<KeyVariant[]>([]);
  const {
    algorithm, setAlgorithm,
    fileName, setFileName,
    comment, setComment,
    passphrase, setPassphrase,
    unencrypted, setUnencrypted,
  } = useGenerationForm();
  const [terminalCommand, setTerminalCommand] = useState<string[] | null>(null);
  const [revealing, setRevealing] = useState<KeyItem | null>(null);
  // 保管庫のフレーズ一覧は、エージェント登録と割り当ての 2 つのフォームが共有する。
  const storedPhrases = useStoredPhrases();
  const {
    phrases, setPhrases,
    setDedicatedPhrasePaths,
    chosenPhrase, setChosenPhrase,
  } = storedPhrases;
  const passphraseForm = usePassphraseForm();
  const {
    setChangingPassphrase,
    currentPassphrase, setCurrentPassphrase,
    newPassphrase, setNewPassphrase,
    removePassphrase,
    close: closePassphraseForm,
  } = passphraseForm;
  const agentForm = useAgentForm(storedPhrases);
  const {
    setRegistering,
    agentPassphrase, setAgentPassphrase,
    agentLifetime,
    close: closeAgentForm,
  } = agentForm;
  const storedPassphraseForm = useStoredPassphraseForm(storedPhrases);
  const {
    setManagingPassphrase,
    storedPhraseName,
    storedPhraseSecret, setStoredPhraseSecret,
    close: closeStoredPassphraseForm,
  } = storedPassphraseForm;
  const [publicKeyView, setPublicKeyView] = useState<{ relativePath: string; text: string } | null>(null);
  const [relocated, setRelocated] = useState<RelocateKeyResponse | null>(null);
  const relocateForm = useRelocateForm();
  const {
    setRelocating,
    newName, setNewName,
    newGroup, setNewGroup,
    createGroup, setCreateGroup,
    close: closeRelocateForm,
  } = relocateForm;
  const [pendingPurge, setPendingPurge] = useState("");
  const [pendingTrash, setPendingTrash] = useState<KeyItem | null>(null);
  const [failure, setFailure] = useState("");
  // 整理の状態。folder は左で開いているもの、chosen は動かす対象、
  // dragging は掴んでいるかどうか（置き場を光らせてよいのはその間だけ）。
  const {
    folder, setFolder,
    chosen, setChosen,
    dragging, setDragging,
    moveOutcome, setMoveOutcome,
    moveTarget, setMoveTarget,
    listFilter, setListFilter,
    keyQuery, setKeyQuery,
    moreActionsFor, setMoreActionsFor,
    selectedKey, setSelectedKey,
  } = useOrganiser();
  const [generated, setGenerated] = useState<{
    private: GeneratedPrivateKeyHandoff;
    public: GeneratedPublicKeyHandoff;
  } | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [nextInventory, nextTrash, nextAlgorithms] = await Promise.all([
        api.inventory(),
        api.listTrash(),
        api.algorithms(),
      ]);
      setInventory(nextInventory);
      setTrash(nextTrash);
      setVariants(nextAlgorithms.variants);
      setState("ready");
    } catch {
      setState("error");
    }
  }, [api]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selected = variants.find((variant) => variant.algorithm === algorithm);
  const inProcess = selected === undefined || selected.inProcess;
  // 描画時に読み取るので、この画面が開いている間に期限が切れた証明書は、
  // 次に何かがそれを更新した時点で「有効」と説明されなくなる。
  const now = Date.now();

  // closeAllForms は、開いている入力欄をすべて畳む。
  //
  // **どの行動もここから始まる。** 別のフォームを開いたまま次を開くと、画面には
  // 二つの「この鍵について」が同時に出る。以前はこの 3 行が行動ごとに書き下されて
  // おり、**1 箇所だけ揃っていなかった** —— 保管庫のパネルを開くところだけが、
  // 自分のフィールドを手で空にして、保管庫の一覧はそのまま持ち越していた。
  //
  // 持ち越しても嘘にはならない（あの一覧は鍵ごとではなく全体のもので、判定は
  // 鍵の綴りで引く）。それでも畳むことにしたのは、**これから取り直すものを根拠に
  // 何かを言わない**方が説明しやすいからである——読み込みが返るまでは何も言わず、
  // 返ってから言う。
  function closeAllForms() {
    closePassphraseForm();
    closeAgentForm();
    closeStoredPassphraseForm();
  }

  // rowActions は、鍵の行から始められることの全部である。
  //
  // **行には setter ではなく意図を渡す。** 「保管庫のパネルを開く」は、開く前に他の
  // 入力欄を畳み、開いたあとに一覧を取り直すところまでを含む——その順序はこの画面の
  // 持ち物であって、行が知っていてよいことではない。
  const rowActions: KeyRowActions = {
    onSelect: (item) => setSelectedKey((current) => (current === item.id ? null : item.id)),
    onToggleChosen: (item, picked) => {
      const next = new Set(chosen);
      if (picked) next.add(item.id);
      else next.delete(item.id);
      setChosen(next);
    },
    onBeginDrag: beginDrag,
    onEndDrag: () => setDragging(false),
    onReveal: (item) => setRevealing(item),
    onShowPublicKey: (item) => void showPublicKey(item),
    onManageStoredPassphrase: (item) => {
      closeAllForms();
      setManagingPassphrase(item);
      void loadPhrases();
    },
    onAddToAgent: (item) => {
      closeAllForms();
      setRegistering(item);
      if (item.encrypted) void loadPhrases();
    },
    onRemoveFromAgent: (item) => {
      closeAllForms();
      void removeFromAgent(item.id);
    },
    onToggleMoreActions: (item) => setMoreActionsFor((current) => (current === item.id ? "" : item.id)),
    onChangePassphrase: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setChangingPassphrase(item);
    },
    onRelocate: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setRelocated(null);
      setNewName(relocateStem(item));
      setNewGroup(groupOfKeyPath(item.relativePath));
      setRelocating(item);
    },
    onMoveToTrash: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setPendingTrash(item);
    },
  };

  // 選ばれた鍵の詳細は右のペインが持つ。消えた鍵のペインは閉じる。
  useEffect(() => {
    if (onInspector === undefined) return;
    const item = inventory?.items.find((candidate) => candidate.id === selectedKey);
    if (item === undefined) {
      onInspector(null);
      return;
    }
    onInspector({
      label: t("keys.inspectorLabel"),
      attention: item.permissionRisk,
      body: <KeyInspector item={item} now={now} />,
    });
  }, [selectedKey, inventory, onInspector, t, now]);

  async function submitGeneration() {
    setFailure("");
    setTerminalCommand(null);
    setGenerated(null);
    try {
      if (selected !== undefined && !selected.inProcess) {
        const response = await api.hardwareCommand({ algorithm, fileName, group: createGroup, comment });
        setTerminalCommand(response.command);
        return;
      }
      const response = await api.generate({
        algorithm,
        bits: selected?.bits ?? 0,
        fileName,
        group: createGroup,
        comment,
        passphrase,
        unencrypted,
      });
      setGenerated({
        private: {
          privateKeyId: response.id,
          privateRelativePath: response.relativePath,
        },
        public: { publicRelativePath: response.publicRelativePath },
      });
      setPassphrase("");
      setFileName("");
      await refresh();
    } catch {
      setPassphrase("");
      setFailure(t("keys.createFailed"));
    }
  }


  // パスフレーズは 1 回の送信の間だけコンポーネント状態にとどまり、
  // 成功時にも失敗時にもクリアされる。他のどこにも保存されることはない。
  async function submitPassphrase(item: KeyItem) {
    setFailure("");
    try {
      await api.changePassphrase(item.id, {
        currentPassphrase,
        newPassphrase: removePassphrase ? "" : newPassphrase,
        unencrypted: removePassphrase,
      });
      closePassphraseForm();
      await refresh();
    } catch {
      setCurrentPassphrase("");
      setNewPassphrase("");
      setFailure(t("keys.passphraseFailed"));
    }
  }

  // エージェントに鍵を返すよう頼む。何も破棄されないので確認は不要だ:
  // 最悪の結果はもう一度パスフレーズを求められることだけだ。応答は
  // その後エージェントが保持しているものを伝え、画面はそれを再読み込みする。
  async function removeFromAgent(keyId: string) {
    try {
      await api.deregisterFromAgent(keyId);
      await refresh();
    } catch {
      setFailure(t("keys.agentRemoveFailed"));
    }
  }


  // 名前はフォームが開いたときに読み込まれ、それより前ではない。起動時には
  // 何も尋ねられず、一度も鍵を登録しない画面は vault に一切触れない。
  // 閉じた vault は何も答えず、それはエラーではなくピッカーなしとして
  // 表示される: このフォームは vault がなくても動作するし、以前からそうだった。
  async function loadPhrases() {
    try {
      const status = await secrets.passwordVault();
      setDedicatedPhrasePaths(status.dedicatedKeyPassphrases);
      if (!status.unlocked) {
        setPhrases([]);
        return;
      }
      const listed = await secrets.credentials();
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
    } catch {
      setPhrases([]);
      setDedicatedPhrasePaths([]);
    }
  }

  async function assignPhrase(item: KeyItem) {
    try {
      const listed = await secrets.assignCredential("key_passphrase", item.relativePath, chosenPhrase);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
      setChosenPhrase("");
    } catch {
      setFailure(t("keys.assignPassphraseFailed"));
    }
  }


  async function storeAndAssignPhrase(item: KeyItem) {
    if (storedPhraseName === "" || storedPhraseSecret === "") return;
    setFailure("");
    // SetCredential は同名の値を置き換える（共有資格情報の rotation）操作でもある。
    // 鍵一覧の「新規保存」でそれを行うと、この名前を使う別の鍵まで黙って変わる。
    // 既存名は上の picker から割り当てさせ、値の更新は用途を一覧できる Secrets
    // 画面だけに限定する。
    if (phrases.some((credential) => credential.name === storedPhraseName)) {
      setStoredPhraseSecret("");
      setFailure(t("keys.storedPassphraseExists"));
      return;
    }
    try {
      await secrets.storeCredential("key_passphrase", storedPhraseName, storedPhraseSecret);
      const listed = await secrets.assignCredential("key_passphrase", item.relativePath, storedPhraseName);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
      closeStoredPassphraseForm();
    } catch {
      // 値は成功時にも失敗時にもDOMから消す。名前は、入力を直せるよう
      // 残してよいが、秘密そのものには同じ扱いをしない。
      setStoredPhraseSecret("");
      setFailure(t("keys.storePassphraseFailed"));
    }
  }

  async function unassignPhrase(item: KeyItem) {
    setFailure("");
    try {
      const listed = await secrets.unassignCredential("key_passphrase", item.relativePath);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
      setChosenPhrase("");
    } catch {
      setFailure(t("keys.unassignPassphraseFailed"));
    }
  }

  // 登録操作は、パスフレーズ変更フォームとまったく同じ長さだけパスフレーズを
  // 保持する: 1 回の送信の間だけで、成功時にも失敗時にもクリアされる。
  // 応答はわざと捨てる——リフレッシュがインベントリを再読み込みするので、
  // その後画面が示すのは、エージェントが報告する保持内容であって、
  // このリクエストが主張した内容ではない。
  async function submitRegistration(item: KeyItem) {
    setFailure("");
    try {
      await api.registerWithAgent(item.id, {
        passphrase: agentPassphrase,
        lifetimeSeconds: agentLifetime,
      });
      closeAgentForm();
      await refresh();
    } catch {
      setAgentPassphrase("");
      setFailure(t("keys.agentFailed"));
    }
  }

  // 公開鍵は秘密ではないので、これは確認も監査記録もない普通の
  // 読み取りだ。コピーされる前に表示されるのは、この画面の他のすべての
  // 値と同じ理由による: クリップボードに載るのは、ユーザーが
  // 見ているものであるべきだ。
  async function showPublicKey(item: KeyItem) {
    setFailure("");
    try {
      const response = await api.publicKey(item.id);
      setPublicKeyView({ relativePath: response.relativePath, text: response.publicKey.trimEnd() });
    } catch {
      setPublicKeyView(null);
      setFailure(t("keys.publicKeyFailed"));
    }
  }

  // moveChosen は選ばれた鍵をひとつの置き場へまとめて移す。
  //
  // **一本が断られても止めない。** relocate は鍵ごとの取引なので、動かせた
  // ものは動かし、動かせなかったものは理由と一緒に名前を出す（moveInto が
  // その仕分けを持つ）。まとめて拒否すると、10 本のうち 1 本のせいで 9 本が
  // 動かないことになる。
  async function moveChosen(target: MoveTarget) {
    if (inventory === null) return;
    const items = inventory.items.filter((item) => chosen.has(item.id));
    if (items.length === 0) return;
    setFailure("");
    const outcome = await moveInto((keyId, change) => api.relocate(keyId, change), items, target);
    setMoveOutcome(outcome);
    setChosen(new Set());
    await refresh();
  }

  // つかんだものが選ばれていなければ、それだけを選ぶ。**掴んだものと動くものを
  // 食い違わせない** —— 選択の外にある行をつかんだのに選択の方が動いたら、
  // 利用者は自分が何を動かしたのかを見ていない。
  function beginDrag(event: DragEvent<HTMLSpanElement>, item: KeyItem) {
    // **何か載せないと、ドラッグが始まらないブラウザがある。** 運ぶのは
    // この文字列ではなく画面の状態の方だが、載せること自体に意味がある。
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", item.relativePath);
    setDragging(true);
    if (!chosen.has(item.id)) setChosen(new Set([item.id]));
  }


  // ブロックされた relocation は、報告して忘れるべき失敗ではない:
  // サーバーは何も書き込まず理由を伝えたので、その理由は画面に残り、
  // フォームはユーザーが入力した内容を残したまま開いたままになる。
  async function submitRelocation(item: KeyItem) {
    setFailure("");
    setRelocated(null);
    try {
      const currentGroup = groupOfKeyPath(item.relativePath);
      const response = await api.relocate(item.id, {
        ...(newName === relocateStem(item) ? {} : { newName }),
        ...(newGroup === currentGroup ? {} : { group: newGroup }),
      });
      setRelocated(response);
      if (response.blockers.length > 0) return;
      closeRelocateForm();
      await refresh();
    } catch {
      setFailure(t("keys.relocateFailed"));
    }
  }

  async function moveToTrash(keyId: string) {
    setFailure("");
    try {
      await api.trash(keyId);
      setPendingTrash(null);
      await refresh();
    } catch {
      setFailure(t("keys.trashFailed"));
    }
  }

  // 鍵はフィンガープリントグループ全体——秘密鍵とその隣の公開鍵——ごと
  // ごみ箱に送られ、それを名指すすべての IdentityFile は
  // 何も指さないまま取り残される。行は既にそれがどのホストかを示していたのに、
  // ボタンは一度もそれに触れず、1 回の押下で全体が実行された。
  function trashGroup(item: KeyItem): KeyItem[] {
    const fingerprint = item.fingerprint;
    if (fingerprint === "") return [item];
    return inventory === null
      ? [item]
      : inventory.items.filter((candidate) => candidate.fingerprint === fingerprint);
  }

  async function restore(entryId: string) {
    setFailure("");
    try {
      const response = await api.restore(entryId);
      if (response.blockers.length > 0) {
        setFailure(t("keys.restoreRefused", { blockers: response.blockers.join(", ") }));
        return;
      }
      await refresh();
    } catch {
      setFailure(t("keys.restoreFailed"));
    }
  }

  async function purge(entryId: string) {
    setFailure("");
    try {
      await api.purge(entryId);
      setPendingPurge("");
      await refresh();
    } catch {
      setFailure(t("keys.purgeFailed"));
    }
  }

  if (state === "loading") {
    return <p aria-live="polite">{t("keys.reading")}</p>;
  }
  if (state === "error" || inventory === null || trash === null) {
    return <p role="alert">{t("keys.unreadable")}</p>;
  }

  const query = keyQuery.trim().toLowerCase();
  const shown = shownItems(inventory.items, listFilter);
  const rows = folderRows(shown, groups);
  const visibleItems = itemsInFolder(shown, folder).filter((item) =>
    query === "" ||
    item.relativePath.toLowerCase().includes(query) ||
    item.kind.toLowerCase().includes(query) ||
    item.algorithm.toLowerCase().includes(query) ||
    item.fingerprint.toLowerCase().includes(query) ||
    item.references.some((reference) =>
      reference.hostPatterns.some((pattern) => pattern.toLowerCase().includes(query)),
    ),
  );
  const keyAttention =
    inventory.unreadable.length +
    inventory.unresolvedReferences.length +
    inventory.items.filter((item) => item.permissionRisk).length;

  return (
    <section className="mx-auto flex w-full max-w-7xl flex-col gap-6">
      <PageHeader
        title={t("keys.heading")}
        description={t("keys.pageDescription")}
        actions={
          <>
            <a href="#create-key-heading" className={secondaryAction}>{t("keys.createHeading")}</a>
            <label>
              <span className="sr-only">{t("keys.listFilter")}</span>
              <select
                className={control}
                value={listFilter}
                onChange={(event) => setListFilter(event.target.value as ListFilter)}
              >
                <option value="keys">{t("keys.listFilterKeys")}</option>
                <option value="all">{t("keys.listFilterAll")}</option>
              </select>
            </label>
            <label className="w-72 max-w-full">
              <span className="sr-only">{t("keys.search")}</span>
              <input
                type="search"
                value={keyQuery}
                onChange={(event) => setKeyQuery(event.target.value)}
                placeholder={t("keys.searchPlaceholder")}
                className={control}
              />
            </label>
          </>
        }
      />
      <MetricGrid>
        <MetricCard label={t("keys.metricFiles")} value={inventory.items.length} />
        <MetricCard
          label={t("keys.metricPrivate")}
          value={inventory.items.filter((item) => item.kind === "private_key").length}
        />
        <MetricCard label={t("keys.metricAttention")} value={keyAttention} attention={keyAttention > 0} />
      </MetricGrid>
      {failure !== "" && (
        <p role="alert" className="rounded-md border border-control-line p-3 text-sm text-danger">
          {failure}
        </p>
      )}

      {chosen.size === 0 ? null : (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-control-line bg-card p-3">
          <p className="grow text-sm text-ink">{t("keys.chosenCount", { count: chosen.size })}</p>
          <select
            aria-label={t("keys.moveTargetLabel")}
            className={control}
            value={moveTarget}
            onChange={(event) => setMoveTarget(event.target.value)}
          >
            <option value="">{t("keys.folderUngrouped")}</option>
            {groups.map((group) => (
              <option key={group} value={group}>
                {group}
              </option>
            ))}
          </select>
          <Button
            kind="primary"
            onClick={() => void moveChosen(moveTarget === "" ? { kind: "ungrouped" } : { kind: "group", name: moveTarget })}
          >
            {t("keys.moveChosen")}
          </Button>
          <Button onClick={() => setChosen(new Set())}>
            {t("keys.clearChosen")}
          </Button>
        </div>
      )}
      {moveOutcome === null ? null : (
        <div role="status" className="rounded-lg border border-line bg-card p-3 text-sm">
          <p className="text-ink">{t("keys.moveMoved", { count: moveOutcome.moved.length })}</p>
          {moveOutcome.blocked.map((entry) => (
            <p key={entry.path} className="text-danger">
              {t("keys.moveBlocked", { path: entry.path, reason: entry.blockers.join(" / ") })}
            </p>
          ))}
          {moveOutcome.failed.map((path) => (
            <p key={path} className="text-danger">
              {t("keys.moveFailed", { path })}
            </p>
          ))}
        </div>
      )}

      <div className="flex flex-col gap-4 md:flex-row">
      <FolderPane
        rows={rows}
        selected={folder}
        dragging={dragging}
        onSelect={(next) => {
          setFolder(next);
          setMoveOutcome(null);
        }}
        onDropInto={(target) => void moveChosen(target)}
      />
      <div className="min-w-0 grow overflow-x-auto rounded-xl border border-line bg-card">
      <KeyTable
        items={visibleItems}
        inventory={inventory}
        chosen={chosen}
        selected={selectedKey}
        moreActionsFor={moreActionsFor}
        now={now}
        actions={rowActions}
      />
      </div>
      {/*
        **ゴミ箱は別の用である。** ここでするのは「戻す」か「完全に消す」で
        あって、いま持っている鍵を整えることではない。畳んであるのは、1000 行
        の一番下に開いたまま置いても誰も辿り着かないからで、**件数だけは畳んだ
        ままでも見える** ——空でないことは、開く前に分かってよい。
      */}
      <details className="rounded-xl border border-line bg-card p-4">
        <summary className="cursor-pointer text-sm font-medium text-ink">
          {t("keys.trashSummary", { count: trash.entries.length })}
        </summary>
        <div className="mt-3 flex flex-col gap-2 border-t border-line pt-3">
        <h3 className={sectionHeading}>{t("keys.trashHeading")}</h3>
        <p className="text-sm text-ink-muted">{t("keys.trashNote")}</p>
        <div className="overflow-x-auto">
        <table className="w-full min-w-[40rem] text-left text-sm">
          <caption className="sr-only">{t("keys.trashCaption")}</caption>
          <thead>
            <tr className={tableHeadRow}>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFiles")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colAge")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colStatus")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {trash.entries.map((entry) => (
              <tr key={entry.id} className="border-b border-line align-top">
                <td className="py-2 pr-3 font-mono text-xs">
                  {entry.files.map((file) => file.originalRelativePath).join(", ")}
                </td>
                <td className="py-2 pr-3">
                  {entry.stale
                    ? t("keys.ageStale", { days: entry.ageDays, retention: trash.retentionDays })
                    : t("keys.age", { days: entry.ageDays })}
                </td>
                <td className="py-2 pr-3">{entry.restorable ? t("keys.restorable") : entry.blockers.join(", ")}</td>
                <td className="py-2">
                  <div className="flex flex-wrap items-center gap-1">
                  <button type="button" className={rowAction} onClick={() => void restore(entry.id)}>
                    {t("keys.restore")}
                  </button>
                  {pendingPurge === entry.id ? (
                    <>
                      <span>{t("keys.purgeWarning")}</span>
                      <button type="button" className={rowDanger} onClick={() => void purge(entry.id)}>
                        {t("keys.confirmPurge")}
                      </button>
                      <button type="button" className={rowAction} onClick={() => setPendingPurge("")}>
                        {t("keys.cancel")}
                      </button>
                    </>
                  ) : (
                    <button type="button" className={rowDanger} onClick={() => setPendingPurge(entry.id)}>
                      {t("keys.purge")}
                    </button>
                  )}
                  </div>
                </td>
              </tr>
            ))}
            {trash.entries.length === 0 && (
              <tr>
                <td colSpan={4} className="py-3 text-sm text-ink-muted">
                  {t("keys.trashEmpty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
        </div>
      </details>
      </div>

      <StoredPassphrasePanel
        form={storedPassphraseForm}
        storedPhrases={storedPhrases}
        onAssign={(item) => void assignPhrase(item)}
        onUnassign={(item) => void unassignPhrase(item)}
        onStoreAndAssign={(item) => void storeAndAssignPhrase(item)}
      />

      <TrashConfirmation
        item={pendingTrash}
        members={pendingTrash === null ? [] : trashGroup(pendingTrash)}
        onConfirm={(id) => void moveToTrash(id)}
        onCancel={() => setPendingTrash(null)}
      />

      {publicKeyView !== null && (
        <section aria-labelledby="public-key-heading" className={sectionCard}>
          <h3 id="public-key-heading" className={sectionHeading}>
            {t("keys.publicKeyHeading", { path: publicKeyView.relativePath })}
          </h3>
          <pre aria-label={t("keys.publicKeyLabel")} className="overflow-x-auto rounded-md bg-canvas p-4 text-xs">
            {publicKeyView.text}
          </pre>
          <div className="flex gap-2">
            <CopyButton value={publicKeyView.text} label="copy.publicKey" />
            <Button onClick={() => setPublicKeyView(null)}>
              {t("keys.close")}
            </Button>
          </div>
        </section>
      )}

      {/*
        スキャナが解釈を拒んだファイルは、以前は単にテーブルから欠落していただけで、
        それは不完全なインベントリを完全なものに見せていた。
        design §6.3 は ~/.ssh 配下のすべてを分類する。これらはそれが
        分類できなかった項目であり、そう言うことが「ここには何もない」と
        「ここには読めなかった何かがある」の違いになる。
      */}
      {inventory.unreadable.length > 0 && (
        <section aria-labelledby="unreadable-heading" className="flex flex-col gap-2">
          <h3 id="unreadable-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unreadableHeading")}
          </h3>
          <p className="text-sm text-ink-muted">{t("keys.unreadableNote")}</p>
          <ul className="text-sm text-ink-muted">
            {inventory.unreadable.map((file) => (
              <li key={file.relativePath}>{t("keys.unreadableEntry", { path: file.relativePath, reason: file.reason })}</li>
            ))}
          </ul>
        </section>
      )}

      {inventory.unresolvedReferences.length > 0 && (
        <section aria-labelledby="unresolved-heading" className="flex flex-col gap-2">
          <h3 id="unresolved-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unresolvedHeading")}
          </h3>
          <ul className="text-sm text-ink-muted">
            {inventory.unresolvedReferences.map((reference) => (
              <li key={`${reference.configPath}:${reference.line}:${reference.value}`}>
                {t("keys.referenceWithReason", {
                  directive: reference.directive,
                  value: reference.value,
                  path: reference.configPath,
                  line: reference.line,
                  reason: reference.reason,
                })}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="agent-heading" className="flex flex-col gap-2">
        <h3 id="agent-heading" className={sectionHeading}>
          {t("keys.agentHeading")}
        </h3>
        {inventory.agentAvailable ? (
          inventory.agentIdentities.length === 0 ? (
            <p className="text-sm text-ink-muted">{t("keys.agentEmpty")}</p>
          ) : (
            <table className="w-full min-w-[32rem] text-left text-sm">
              <caption className="sr-only">{t("keys.agentIdentitiesCaption")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colAlgorithm")}</th>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFingerprint")}</th>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colComment")}</th>
                </tr>
              </thead>
              <tbody>
                {inventory.agentIdentities.map((identity) => (
                  <tr key={identity.fingerprint} className="border-b border-line">
                    <td className="py-2 pr-3">
                      {identity.bits > 0 ? `${identity.algorithm} · ${identity.bits}` : identity.algorithm}
                    </td>
                    <td className="py-2 pr-3 font-mono text-xs break-all">{identity.fingerprint}</td>
                    <td className="py-2">{identity.comment}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          // ssh-add は SSH_AUTH_SOCK が指すものと話す。「エージェントが動いていない」
          // と言うのは推測になる: このプロセスに単にソケットが渡されて
          // いないだけかもしれない。メッセージは何が足りないかを言い、理由は言わない。
          <p className="text-sm text-notice-ink">
            {t("keys.agentUnavailable")}
          </p>
        )}
        {inventory.agentDelegations.length > 0 && (
          <>
            <p className="text-sm text-ink-muted">
              {t("keys.agentDelegationsNote")}
            </p>
            <ul className="text-sm text-ink-muted">
              {inventory.agentDelegations.map((reference) => (
                <li key={`${reference.configPath}:${reference.line}`}>
                  {t("keys.reference", {
                    directive: reference.directive,
                    value: reference.value,
                    path: reference.configPath,
                    line: reference.line,
                  })}
                  {reference.hostPatterns.length > 0 ? ` (${reference.hostPatterns.join(" ")})` : ""}
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <AgentForm
        form={agentForm}
        storedPhrases={storedPhrases}
        onSubmit={(item) => void submitRegistration(item)}
        onAssignPhrase={(item) => void assignPhrase(item)}
      />

      <RelocateForm form={relocateForm} groups={groups} onSubmit={(item) => void submitRelocation(item)} />

      {/*
        リネームが何をしたか、あるいは何がそれを止めたか。どちらも同じ種類の事実の
        リストだ: どのファイルが移動したか、どの設定行が書き換えられたか、そして
        このアプリケーションが意図的に触れなかったものは何か。「完了」とだけ言う
        リネームは、ユーザー自身では確認できない部分を隠してしまう。
      */}
      <RelocateResult result={relocated} onClose={() => setRelocated(null)} />

      {revealing !== null && (
        <RevealDialog
          keyId={revealing.id}
          relativePath={revealing.relativePath}
          api={api}
          onClose={() => setRevealing(null)}
        />
      )}

      <PassphraseForm form={passphraseForm} onSubmit={(item) => void submitPassphrase(item)} />

      <form
        aria-labelledby="create-key-heading"
        className="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void submitGeneration();
        }}
      >
        <h3 id="create-key-heading" className={sectionHeading}>
          {t("keys.createHeading")}
        </h3>
        <Card>
          <Row label={t("keys.algorithm")}>
            <select className={control} value={algorithm} onChange={(event) => setAlgorithm(event.target.value)}>
              {variants.map((variant) => (
                <option key={`${variant.algorithm}-${variant.bits}`} value={variant.algorithm}>
                  {variant.label}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.createGroup")}>
            <select className={control} value={createGroup} onChange={(event) => setCreateGroup(event.target.value)}>
              <option value="">{t("keys.groupNone")}</option>
              {groups.map((group) => (
                <option key={group} value={group}>
                  {group}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.fileName")}>
            <input className={control} value={fileName} onChange={(event) => setFileName(event.target.value)} />
          </Row>
          <Row label={t("keys.comment")}>
            <input className={control} value={comment} onChange={(event) => setComment(event.target.value)} />
          </Row>
          {inProcess && (
            <Row label={t("keys.passphrase")}>
              <input
                className={control}
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                disabled={unencrypted}
              />
            </Row>
          )}
        </Card>
        {inProcess && (
          <CheckboxField
            label={t("keys.createUnencrypted")}
            checked={unencrypted}
            onChange={(checked) => {
              setUnencrypted(checked);
              setPassphrase("");
            }}
          />
        )}
        <Button kind="primary" type="submit" className="self-start">
          {inProcess ? t("keys.createSubmit") : t("keys.showTerminalCommand")}
        </Button>
      </form>

      {generated === null ? null : (
        <section aria-live="polite" className={sectionCard}>
          <h3 className={sectionHeading}>{t("keys.generatedHeading")}</h3>
          <p className={hintText}>
            {t("keys.generatedNext", { path: generated.private.privateRelativePath })}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              kind="primary"
              onClick={() => onAssignGeneratedKey?.(generated.private)}
            >
              {t("keys.assignGenerated")}
            </Button>
            <Button
              onClick={() => onInstallGeneratedKey?.(generated.public)}
            >
              {t("keys.installGenerated")}
            </Button>
          </div>
        </section>
      )}

      {terminalCommand !== null && (
        <div>
          <p className="text-sm text-ink-muted">
            {t("keys.hardwareNote")}
          </p>
          <pre aria-label={t("copy.terminalCommand")} className="overflow-x-auto rounded-md bg-canvas p-4 text-xs">
            {terminalCommand.join(" ")}
          </pre>
          <div className="mt-2">
            <CopyButton value={terminalCommand.join(" ")} label="copy.terminalCommand" />
          </div>
        </div>
      )}

    </section>
  );
}
