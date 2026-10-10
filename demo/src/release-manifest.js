// An upper bound rejects a broken manifest before allocating an archive in the browser.
export const maximumReleaseBytes = 128 * 1024 * 1024;

export function validateReleaseManifest(manifest, proxyURL) {
  if (manifest.format !== 1 || !/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(manifest.version) ||
      manifest.fileName !== `sshc-demo-${manifest.version}.tar.gz` || !/^[a-f0-9]{64}$/.test(manifest.sha256) ||
      !Number.isSafeInteger(manifest.bytes) || manifest.bytes <= 0 || manifest.bytes > maximumReleaseBytes ||
      !Number.isSafeInteger(manifest.fileCount) || manifest.fileCount <= 0) {
    throw new Error("Invalid release manifest");
  }
  const archiveURL = new URL(manifest.archiveURL);
  if (archiveURL.origin !== proxyURL.origin || archiveURL.pathname !== `/archives/${manifest.version}.tar.gz` ||
      archiveURL.search || archiveURL.hash) throw new Error("Invalid release archive URL");
  return { ...manifest, archiveURL };
}
