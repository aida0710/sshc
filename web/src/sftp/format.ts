export function formatBytes(value: number): string {
  if (value < 1024) return `${Math.max(0, value).toLocaleString()} B`;
  if (value < 1 << 20) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1 << 30) return `${(value / (1 << 20)).toFixed(1)} MiB`;
  return `${(value / (1 << 30)).toFixed(1)} GiB`;
}
