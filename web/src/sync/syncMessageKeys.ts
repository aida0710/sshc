import type { SyncDirection, SyncHistoryRevision, SyncSetupCheckResponse } from "../api/sync";
import type { MessageKey } from "../i18n/messages";

// The wording for each value the engine reports about sync. Each key is written
// out in full so that the catalogue test can tell which keys are still shown;
// the Record type makes a new engine value fail to compile until it has one.

export const syncDirectionLabelKeys: Record<SyncDirection, MessageKey> = {
  both: "sync.direction.both",
  push: "sync.direction.push",
  pull: "sync.direction.pull",
};

export const syncDirectionHintKeys: Record<SyncDirection, MessageKey> = {
  both: "sync.direction.both.hint",
  push: "sync.direction.push.hint",
  pull: "sync.direction.pull.hint",
};

export const syncAutoHintKeys: Record<SyncDirection, MessageKey> = {
  both: "sync.autoHint.both",
  push: "sync.autoHint.push",
  pull: "sync.autoHint.pull",
};

export const syncNowLabelKeys: Record<SyncDirection, MessageKey> = {
  both: "sync.autoNow.both",
  push: "sync.autoNow.push",
  pull: "sync.autoNow.pull",
};

export const syncSetupStateKeys: Record<SyncSetupCheckResponse["state"], MessageKey> = {
  empty: "sync.setup.empty",
  existing: "sync.setup.existing",
  incomplete: "sync.setup.incomplete",
};

export const syncHistoryRelationLabelKeys: Record<SyncHistoryRevision["relation"], MessageKey> = {
  head: "sync.historyRelation.head",
  ancestor: "sync.historyRelation.ancestor",
  branch: "sync.historyRelation.branch",
};
