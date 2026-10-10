// v86 loads external initrds at 64 MiB; servers need room above that address for the image.
export const machineDefinitions = [
  { role: "client", label: "sshc", purpose: "clientPurpose", memoryMiB: 192, image: "client.cpio.gz" },
  { role: "demo-a", label: "demo-a", purpose: "serverPurpose", memoryMiB: 128, image: "server.cpio.gz" },
  { role: "demo-b", label: "demo-b", purpose: "serverPurpose", memoryMiB: 128, image: "server.cpio.gz" },
];

// Each file counts once: VMs that share a boot file or image read the browser's cached copy.
export function bundleDownloadBytes({ assetSizes, uiArchive }) {
  return Object.values(assetSizes).reduce((sum, bytes) => sum + bytes, 0) + (uiArchive?.bytes ?? 0);
}

export async function loadDemoConfiguration() {
  const response = await fetch(new URL("./config.json", window.location.href));
  if (!response.ok) throw new Error("Demo configuration is unavailable");
  const config = await response.json();
  return { imageBaseURL: new URL(config.imageBaseURL, window.location.href),
    version: config.version, releaseProxyURL: config.releaseProxyURL,
    assetSizes: config.assetSizes ?? {}, uiArchive: config.uiArchive,
    entryURL: new URL(config.entryURL ?? "./index.html", window.location.href) };
}
