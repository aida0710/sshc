import type { Notice } from "../api/config";
import { folderNoticesOf } from "./groupFolderContents";

// The kinds of configuration notice, by where a person meets them. Connections shows each kind in its place;
// Home counts only the ones that ask the person to fix something, and sends them to that place.

// groupNoticeCodes are about group folders. Connections shows them over the list of groups (folderNoticesOf).
const groupNoticeCodes = new Set([
  "group_not_declared",
  "group_directory_missing",
  "group_empty",
  "group_directory_leftover",
  "group_file_unreached",
]);

// selectionNoticeCodes explain how one connection's values come about (a `Host *` that also applies, a block
// with no name of its own). Connections shows them when that connection is opened; they ask for nothing.
const selectionNoticeCodes = new Set([
  "complex_external_rule",
  "wildcard_shadow",
  "negated_pattern",
  "unnamed_host_block",
  "match_block",
  "dangerous_directive",
  "explained_values_only",
]);

// configurationNoticesOf is what Connections shows beside the list: the notices about the configuration that
// are neither about a group folder nor about how one connection's values come about.
export function configurationNoticesOf(notices: readonly Notice[]): Notice[] {
  return notices.filter((notice) => !groupNoticeCodes.has(notice.code) && !selectionNoticeCodes.has(notice.code));
}

// attentionNoticesOf is every notice Connections shows with nothing opened: the configuration's and the
// group folders' that need fixing.
export function attentionNoticesOf(notices: readonly Notice[]): Notice[] {
  return [...configurationNoticesOf(notices), ...folderNoticesOf(notices)];
}
