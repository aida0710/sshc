import type { components, operations } from "../api/schema";
import type { SFTPLocation } from "./sftpLocation";

export type SearchMode = NonNullable<operations["searchSFTPEntries"]["parameters"]["query"]["mode"]>;
export type ComparisonMode = NonNullable<components["schemas"]["SFTPDirectoryComparison"]["mode"]>;
export type DirectoryCompareOptions = {
  left: SFTPLocation;
  right: SFTPLocation;
  mode?: ComparisonMode;
  signal?: AbortSignal;
};
export type SearchOptions = SFTPLocation & {
  query: string;
  mode?: SearchMode;
  signal?: AbortSignal;
};
