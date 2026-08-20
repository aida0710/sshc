import { useCallback, useState } from "react";
import type { Credential } from "../api/integrations";
import type { KeyItem } from "./api";
import type { Folder, ListFilter, MoveOutcome } from "./organizer";

// 鍵の画面が抱えるワークフローごとの状態である。
//
// **以前は 42 個の useState が同じスコープに並んでいた。** どの操作がどれを触るのかは
// 1600 行を読まないと分からず、実際 closeAgentForm と closeStoredPassphraseForm は
// 同じ 3 つを消していた——保管庫のフレーズ一覧が、2 つのフォームの間で暗黙に
// 共有されていたのである。ここではそれを共有物として名前を付けた。
//
// **返す名前は画面が使っていたものに合わせてある。** 束ね直しただけで、どの操作が
// 何を触るかは変えていない。

// useStoredPhrases は、保管庫が持っているパスフレーズの一覧である。
//
// **2 つのフォームがこれを使う。** エージェント登録と、鍵へのパスフレーズ割り当て
// である。どちらもフォームを開いたときに読み込み、閉じたときに捨てる——起動時には
// 何も尋ねられず、一度も使わない画面は vault に一切触れない。
export function useStoredPhrases() {
  const [phrases, setPhrases] = useState<Credential[]>([]);
  const [dedicatedPhrasePaths, setDedicatedPhrasePaths] = useState<string[]>([]);
  const [chosenPhrase, setChosenPhrase] = useState("");
  const reset = useCallback(() => {
    setChosenPhrase("");
    setPhrases([]);
    setDedicatedPhrasePaths([]);
  }, []);
  return {
    phrases, setPhrases,
    dedicatedPhrasePaths, setDedicatedPhrasePaths,
    chosenPhrase, setChosenPhrase,
    reset,
  };
}

// usePassphraseForm は、鍵のパスフレーズを変える／外すフォームである。
//
// **入力は 1 回の送信の間だけここに居る。** 成功しても失敗しても消え、他のどこにも
// 保存されない。
export function usePassphraseForm() {
  const [changingPassphrase, setChangingPassphrase] = useState<KeyItem | null>(null);
  const [currentPassphrase, setCurrentPassphrase] = useState("");
  const [newPassphrase, setNewPassphrase] = useState("");
  const [removePassphrase, setRemovePassphrase] = useState(false);
  const close = useCallback(() => {
    setCurrentPassphrase("");
    setNewPassphrase("");
    setRemovePassphrase(false);
    setChangingPassphrase(null);
  }, []);
  return {
    changingPassphrase, setChangingPassphrase,
    currentPassphrase, setCurrentPassphrase,
    newPassphrase, setNewPassphrase,
    removePassphrase, setRemovePassphrase,
    close,
  };
}

// useAgentForm は、鍵を ssh-agent へ登録するフォームである。
export function useAgentForm(storedPhrases: { reset: () => void }) {
  const [registering, setRegistering] = useState<KeyItem | null>(null);
  const [agentPassphrase, setAgentPassphrase] = useState("");
  const [agentLifetime, setAgentLifetime] = useState(0);
  const close = useCallback(() => {
    setAgentPassphrase("");
    setAgentLifetime(0);
    storedPhrases.reset();
    setRegistering(null);
  }, [storedPhrases]);
  return {
    registering, setRegistering,
    agentPassphrase, setAgentPassphrase,
    agentLifetime, setAgentLifetime,
    close,
  };
}

// useStoredPassphraseForm は、鍵に保管庫のパスフレーズを割り当てるフォームである。
export function useStoredPassphraseForm(storedPhrases: { reset: () => void }) {
  const [managingPassphrase, setManagingPassphrase] = useState<KeyItem | null>(null);
  const [storedPhraseName, setStoredPhraseName] = useState("");
  const [storedPhraseSecret, setStoredPhraseSecret] = useState("");
  const close = useCallback(() => {
    setStoredPhraseName("");
    setStoredPhraseSecret("");
    storedPhrases.reset();
    setManagingPassphrase(null);
  }, [storedPhrases]);
  return {
    managingPassphrase, setManagingPassphrase,
    storedPhraseName, setStoredPhraseName,
    storedPhraseSecret, setStoredPhraseSecret,
    close,
  };
}

// useRelocateForm は、鍵の名前を変える／別のグループへ移すフォームである。
//
// **relocated は閉じても消えない。** 断られた relocation は、報告して忘れるべき失敗
// ではない——サーバーは何も書かずに理由を伝えたので、その理由は画面に残る。
export function useRelocateForm() {
  const [relocating, setRelocating] = useState<KeyItem | null>(null);
  const [newName, setNewName] = useState("");
  const [newGroup, setNewGroup] = useState("");
  const [createGroup, setCreateGroup] = useState("");
  const close = useCallback(() => {
    setNewName("");
    setNewGroup("");
    setRelocating(null);
  }, []);
  return {
    relocating, setRelocating,
    newName, setNewName,
    newGroup, setNewGroup,
    createGroup, setCreateGroup,
    close,
  };
}

// useGenerationForm は、新しい鍵を作るフォームである。
//
// **passphrase はここに留まる。** 生成の 1 回のあいだだけ持ち、送ったら消える。
export function useGenerationForm() {
  const [algorithm, setAlgorithm] = useState("ed25519");
  const [fileName, setFileName] = useState("");
  const [comment, setComment] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [unencrypted, setUnencrypted] = useState(false);
  return {
    algorithm, setAlgorithm,
    fileName, setFileName,
    comment, setComment,
    passphrase, setPassphrase,
    unencrypted, setUnencrypted,
  };
}

// useOrganiser は、一覧の見え方と選択である。
//
// **フォルダも選択もサーバーには無い。** どちらもこの画面が、鍵の相対パスから
// 組み立てて見せているものである。
export function useOrganiser() {
  const [folder, setFolder] = useState<Folder>({ kind: "all" });
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const [dragging, setDragging] = useState(false);
  const [moveOutcome, setMoveOutcome] = useState<MoveOutcome | null>(null);
  const [moveTarget, setMoveTarget] = useState("");
  const [listFilter, setListFilter] = useState<ListFilter>("keys");
  const [keyQuery, setKeyQuery] = useState("");
  const [moreActionsFor, setMoreActionsFor] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  return {
    folder, setFolder,
    chosen, setChosen,
    dragging, setDragging,
    moveOutcome, setMoveOutcome,
    moveTarget, setMoveTarget,
    listFilter, setListFilter,
    keyQuery, setKeyQuery,
    moreActionsFor, setMoreActionsFor,
    selectedKey, setSelectedKey,
  };
}
