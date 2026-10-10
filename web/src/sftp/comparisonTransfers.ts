import type { DirectoryComparison } from "./api";
import type { SFTPLocation } from "./sftpLocation";
import type { RemoteTransferSelection } from "./transferManager";

export function comparisonTransfers({ comparison, selected, direction, left, right }: {
  comparison: DirectoryComparison;
  selected: Set<string>;
  direction: "left" | "right";
  left: SFTPLocation;
  right: SFTPLocation;
}): RemoteTransferSelection[] {
  const sourceRoot = direction === "left" ? comparison.leftPath : comparison.rightPath;
  const targetRoot = direction === "left" ? comparison.rightPath : comparison.leftPath;
  const sourceAlias = direction === "left" ? left.alias : right.alias;
  const targetAlias = direction === "left" ? right.alias : left.alias;
  const candidates = comparison.entries.filter((difference) => selected.has(difference.relativePath) && difference.status !== "same" && difference.status !== "unverified")
    .filter((difference) => (direction === "left" ? difference.left : difference.right) !== undefined);
  // A directory transfer already includes its descendants; avoid duplicate jobs.
  const directories = candidates.flatMap((difference) => (direction === "left" ? difference.left : difference.right)?.type === "directory" ? [difference.relativePath] : []);
  return candidates.flatMap((difference) => {
    const source = direction === "left" ? difference.left : difference.right;
    if (source === undefined || directories.some((directory) => difference.relativePath !== directory && difference.relativePath.startsWith(`${directory}/`))) return [];
    return [{
      sourceAlias, targetAlias,
      sourcePath: `${sourceRoot === "/" ? "" : sourceRoot}/${difference.relativePath}`,
      targetPath: `${targetRoot === "/" ? "" : targetRoot}/${difference.relativePath}`,
      kind: source.type === "directory" ? "folder" : "file",
      name: source.name,
      totalBytes: source.type === "file" ? source.size : -1,
      overwrite: difference.status !== (direction === "left" ? "left_only" : "right_only"),
    }];
  });
}
