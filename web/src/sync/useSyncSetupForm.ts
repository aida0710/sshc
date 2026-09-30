import { useEffect, useState } from "react";
import type { SyncDirection, SyncSetupCheckResponse, SyncSetupRequest, SyncStatus } from "../api/sync";

type SetupFormState = {
  endpoint: string;
  bucket: string;
  path: string;
  region: string;
  accessKeyId: string;
  secretAccessKey: string;
  direction: SyncDirection;
  setupCheck: SyncSetupCheckResponse | null;
  ownKey: string;
  chooseOwn: boolean;
  confirmHistoryLoss: boolean;
  editingSettings: boolean;
  settingsOpen: boolean;
};

const initialState: SetupFormState = {
  endpoint: "",
  bucket: "",
  path: "",
  region: "",
  accessKeyId: "",
  secretAccessKey: "",
  direction: "both",
  setupCheck: null,
  ownKey: "",
  chooseOwn: false,
  confirmHistoryLoss: false,
  editingSettings: false,
  settingsOpen: false,
};

export function useSyncSetupForm() {
  const [state, setState] = useState<SetupFormState>(initialState);

  function set<K extends keyof SetupFormState>(
    key: K,
    value: SetupFormState[K],
  ) {
    setState((current) => ({ ...current, [key]: value }));
  }

  useEffect(() => {
    setState((current) =>
      current.setupCheck === null ? current : { ...current, setupCheck: null },
    );
  }, [
    state.endpoint,
    state.bucket,
    state.path,
    state.region,
    state.accessKeyId,
    state.secretAccessKey,
  ]);

  function editSettings(current: SyncStatus) {
    setState((form) => ({
      ...form,
      endpoint: current.endpoint ?? "",
      bucket: current.bucket ?? "",
      path: current.path ?? "",
      region: current.region ?? "",
      direction: current.direction,
      accessKeyId: "",
      secretAccessKey: "",
      setupCheck: null,
      editingSettings: true,
      settingsOpen: true,
    }));
  }

  const setupInput = {
    endpoint: state.endpoint,
    bucket: state.bucket,
    path: state.path,
    region: state.region,
    accessKeyId: state.accessKeyId,
    secretAccessKey: state.secretAccessKey,
    reuseCredentials: false,
  };

  // A checked bucket starts the key choice over: the check says whether the
  // bucket already has a key to type in or needs a new one.
  function acceptSetupCheck(check: SyncSetupCheckResponse) {
    setState((form) => ({ ...form, setupCheck: check, ownKey: "", chooseOwn: false }));
  }

  // setupRequest は、確かめたバケットの状態を添えて、設定を保存する要求を作る。
  // 確かめる前は null である。既存のバケットか、自分で鍵を選んだときだけ入力した鍵を送る。
  function setupRequest(): SyncSetupRequest | null {
    const { setupCheck, chooseOwn, ownKey } = state;
    if (setupCheck === null) return null;
    return {
      ...setupInput,
      direction: state.direction,
      expectedState: setupCheck.state,
      ...(setupCheck.etag === undefined ? {} : { expectedETag: setupCheck.etag }),
      historyPresent: setupCheck.historyPresent,
      reuseKey: false,
      key: setupCheck.state === "existing" || chooseOwn ? ownKey : "",
    };
  }

  // 保存できたら、入力した鍵と認証情報を消して設定を閉じる。
  function finishSetup() {
    setState((form) => ({
      ...form,
      ownKey: "",
      accessKeyId: "",
      secretAccessKey: "",
      setupCheck: null,
      editingSettings: false,
      settingsOpen: false,
    }));
  }

  return {
    ...state,
    setupInput,
    editSettings,
    acceptSetupCheck,
    setupRequest,
    finishSetup,
    setEndpoint: (value: string) => set("endpoint", value),
    setBucket: (value: string) => set("bucket", value),
    setPath: (value: string) => set("path", value),
    setRegion: (value: string) => set("region", value),
    setAccessKeyId: (value: string) => set("accessKeyId", value),
    setSecretAccessKey: (value: string) => set("secretAccessKey", value),
    setDirection: (value: SyncDirection) => set("direction", value),
    setSetupCheck: (value: SyncSetupCheckResponse | null) =>
      set("setupCheck", value),
    setOwnKey: (value: string) => set("ownKey", value),
    setChooseOwn: (value: boolean) => set("chooseOwn", value),
    setConfirmHistoryLoss: (value: boolean) => set("confirmHistoryLoss", value),
    setEditingSettings: (value: boolean) => set("editingSettings", value),
    setSettingsOpen: (value: boolean) => set("settingsOpen", value),
  };
}

export type SyncSetupForm = ReturnType<typeof useSyncSetupForm>;
