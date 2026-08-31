import { useState } from "react";
import type { PullResponse } from "../api/integrations";

type Resolution = "local" | "remote" | undefined;

type PullPreviewState = {
  preview: PullResponse | null;
  historyKey: string | undefined;
  acceptRemoteHead: boolean;
  acceptedRemovals: boolean;
  resolve: Resolution;
};

const initialState: PullPreviewState = {
  preview: null,
  historyKey: undefined,
  acceptRemoteHead: false,
  acceptedRemovals: false,
  resolve: undefined,
};

export function useSyncPullPreview() {
  const [state, setState] = useState<PullPreviewState>(initialState);

  return {
    ...state,
    prepare: (resolve: Resolution, historyKey?: string) =>
      setState((current) => ({
        ...current,
        resolve,
        historyKey,
        acceptRemoteHead: false,
      })),
    prepareRemoteHead: () =>
      setState((current) => ({
        ...current,
        resolve: "remote",
        historyKey: undefined,
        acceptRemoteHead: true,
      })),
    show: (preview: PullResponse) =>
      setState((current) => ({
        ...current,
        preview,
        acceptedRemovals: false,
      })),
    replace: (preview: PullResponse) =>
      setState((current) => ({ ...current, preview })),
    close: () => setState((current) => ({ ...current, preview: null })),
    acceptRemovals: (accepted: boolean) =>
      setState((current) => ({ ...current, acceptedRemovals: accepted })),
  };
}
