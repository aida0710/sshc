import type { MessageKey } from "../i18n/messages";
import type { DirectoryComparison, RemoteEntry } from "./api";
import type { SFTPSort } from "./sftpEntrySort";

// The wording for each value the SFTP screen shows. Each key is written out in
// full so that the catalogue test can tell which keys are still shown; the
// Record type makes a new engine value fail to compile until it has one.

export const entryTypeLabelKeys: Record<RemoteEntry["type"], MessageKey> = {
  file: "sftp.type.file",
  directory: "sftp.type.directory",
  symlink: "sftp.type.symlink",
  other: "sftp.type.other",
};

export const sortColumnLabelKeys: Record<SFTPSort, MessageKey> = {
  name: "sftp.name",
  type: "sftp.type",
  size: "sftp.size",
  modified: "sftp.modified",
  uid: "sftp.uid",
  gid: "sftp.gid",
};

export const comparisonStatusLabelKeys: Record<DirectoryComparison["entries"][number]["status"], MessageKey> = {
  same: "sftp.compare.same",
  different: "sftp.compare.different",
  left_only: "sftp.compare.left_only",
  right_only: "sftp.compare.right_only",
  type_mismatch: "sftp.compare.type_mismatch",
  unverified: "sftp.compare.unverified",
};
