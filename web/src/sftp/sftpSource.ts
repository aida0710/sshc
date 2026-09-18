import { sftpApi, type RemoteEntry } from "./api";
import { localHostAlias } from "./localHost";

// A source is somewhere a pane can browse: an SSH host over SFTP, or the
// filesystem of the machine the sshc engine runs on. The pane never asks which
// one it has. It asks the source how to list, how paths nest and what the
// source is able to do, and shows or hides its operations from that.

export type SFTPCrumb = { label: string; path: string };
export type SFTPListing = { path: string; entries: RemoteEntry[]; home?: string };

export type SFTPCapabilities = {
  // Wait for an explicit "connect" before the first listing. An SSH session
  // costs a handshake and a host-key check; the engine's own disk does not.
  connect: boolean;
  edit: boolean;
  createEntries: boolean;
  rename: boolean;
  chmod: boolean;
  delete: boolean;
  search: boolean;
  details: boolean;
  // Files chosen or dropped from the browser can be sent here.
  browserUpload: boolean;
  download: boolean;
  // Rows can be dragged out to another pane.
  dragOut: boolean;
  terminal: boolean;
};

export type SFTPSource = {
  alias: string;
  local: boolean;
  can: SFTPCapabilities;
  list(path: string): Promise<SFTPListing>;
  parentOf(path: string): string;
  join(directory: string, name: string): string;
  isRoot(path: string): boolean;
  rootOf(path: string): string;
  // The root first, the current directory last. `home` lets a source label
  // the user's home as "~".
  crumbs(path: string, home?: string): SFTPCrumb[];
  // Whether a remembered or linked path could belong to this source at all.
  acceptsPath(path: string): boolean;
};

export function remoteParentOf(remotePath: string): string {
  if (remotePath === "/") return "/";
  const pieces = remotePath.split("/").filter(Boolean);
  pieces.pop();
  return `/${pieces.join("/")}` || "/";
}

export function remoteJoin(parent: string, name: string): string {
  return `${parent === "/" ? "" : parent}/${name}`;
}

const remoteCapabilities: SFTPCapabilities = {
  connect: true, edit: true, createEntries: true, rename: true, chmod: true, delete: true, search: true,
  details: true, browserUpload: true, download: true, dragOut: true, terminal: true,
};

export function remoteSource(alias: string): SFTPSource {
  return {
    alias,
    local: false,
    can: remoteCapabilities,
    list: (path) => sftpApi.list(alias, path),
    parentOf: remoteParentOf,
    join: remoteJoin,
    isRoot: (path) => path === "/",
    rootOf: () => "/",
    crumbs(path) {
      const pieces = path.split("/").filter(Boolean);
      return [{ label: "/", path: "/" }, ...pieces.map((piece, index) => ({ label: piece, path: `/${pieces.slice(0, index + 1).join("/")}` }))];
    },
    acceptsPath: (path) => path.startsWith("/"),
  };
}

// Engine-local paths are always forward-slashed, but their root can be "/",
// a Windows drive such as "C:/", or a network share "//server/share/".
export function localRootOf(value: string): string {
  const unc = /^\/\/[^/]+\/[^/]+(?:\/|$)/.exec(value);
  if (unc) return `${unc[0].replace(/\/$/, "")}/`;
  return /^[A-Za-z]:\//.test(value) ? value.slice(0, 3) : "/";
}

export function localParentOf(value: string): string {
  const normalized = value.replace(/\\/g, "/");
  const root = localRootOf(normalized);
  if (normalized === root || normalized === root.replace(/\/$/, "")) return value;
  const withoutTrailingSlash = normalized.replace(/\/$/, "");
  const index = withoutTrailingSlash.lastIndexOf("/");
  if (index < root.length) return root;
  return withoutTrailingSlash.slice(0, index);
}

export function localJoin(parent: string, name: string): string {
  return `${parent.replace(/\/$/, "")}/${name}`;
}

// The engine's disk offers no editing and takes no files from the browser,
// but its rows can be dragged onto a host: that drop is the engine-side put.
const localCapabilities: SFTPCapabilities = {
  connect: false, edit: false, createEntries: false, rename: false, chmod: false, delete: false, search: false,
  details: false, browserUpload: false, download: false, dragOut: true, terminal: false,
};

export const localSource: SFTPSource = {
  alias: localHostAlias,
  local: true,
  can: localCapabilities,
  list: (path) => sftpApi.listLocal(path),
  parentOf: localParentOf,
  join: localJoin,
  isRoot: (path) => path === localRootOf(path),
  rootOf: localRootOf,
  crumbs(value, home) {
    const normalized = value.replace(/\\/g, "/");
    const root = localRootOf(normalized);
    const label = (crumb: SFTPCrumb) => crumb.path === home ? { ...crumb, label: "~" } : crumb;
    if (normalized === root) return [label({ label: root, path: root })];
    const parts = normalized.slice(root.length).split("/").filter(Boolean);
    return [
      label({ label: root, path: root }),
      ...parts.map((part, index) => label({ label: part, path: `${root}${parts.slice(0, index + 1).join("/")}` })),
    ];
  },
  acceptsPath: (path) => path.startsWith("/") || /^[A-Za-z]:\//.test(path),
};

export function sourceFor(alias: string): SFTPSource | null {
  if (alias === "") return null;
  return alias === localHostAlias ? localSource : remoteSource(alias);
}
