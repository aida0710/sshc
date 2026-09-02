import { useEffect, useState } from "react";
import type {
  SyncDirection,
  SyncSetupCheckResponse,
  SyncStatus,
} from "../api/integrations";

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

  return {
    ...state,
    setupInput,
    editSettings,
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
