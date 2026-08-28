import { useCallback, useState } from "react";
import type { Credential } from "../api/integrations";
import type { KeyItem } from "./api";
import type { Folder, ListFilter, MoveOutcome } from "./organizer";


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

export function useOrganiser() {
  const [folder, setFolder] = useState<Folder>({ kind: "all" });
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const [dragging, setDragging] = useState(false);
  const [moveOutcome, setMoveOutcome] = useState<MoveOutcome | null>(null);
  const [moveTarget, setMoveTarget] = useState("");
  const [listFilter, setListFilter] = useState<ListFilter>("keys");
  const [keyQuery, setKeyQuery] = useState("");
  const [detailsFor, setDetailsFor] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  return {
    folder, setFolder,
    chosen, setChosen,
    dragging, setDragging,
    moveOutcome, setMoveOutcome,
    moveTarget, setMoveTarget,
    listFilter, setListFilter,
    keyQuery, setKeyQuery,
    detailsFor, setDetailsFor,
    selectedKey, setSelectedKey,
  };
}
