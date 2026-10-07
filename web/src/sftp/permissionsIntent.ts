import type { RemoteEntry } from "./api";
import type { ChmodPlan, ChmodSelection } from "./chmodApi";

export type PermissionsIntent = {
  entries: RemoteEntry[];
  alias: string;
  directory: string;
  recursive: boolean;
  initialMode?: string;
  isCurrent: () => boolean;
};

export type ConfirmedPermissions = { selection: ChmodSelection; plan: ChmodPlan };
