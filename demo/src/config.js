// v86 loads external initrds at 64 MiB; servers need room above that address for the image.
export const machineDefinitions = [
  { role: "client", label: "sshc", purpose: "clientPurpose", memoryMiB: 192, image: "client.cpio.gz" },
  { role: "demo-a", label: "demo-a", purpose: "serverPurpose", memoryMiB: 128, image: "server.cpio.gz" },
  { role: "demo-b", label: "demo-b", purpose: "serverPurpose", memoryMiB: 128, image: "server.cpio.gz" },
];

export async function loadDemoConfiguration() {
  const response = await fetch(new URL("./config.json", window.location.href));
  if (!response.ok) throw new Error("Demo configuration is unavailable");
  const config = await response.json();
  return { imageBaseURL: new URL(config.imageBaseURL, window.location.href),
    assetSizes: config.assetSizes ?? {}, uiArchive: config.uiArchive,
    entryURL: new URL(config.entryURL ?? "./index.html", window.location.href) };
}
