// relativeConfigPath shows a configuration file by its path in the folder of the root configuration file
// (rootConfig, normally ~/.ssh/config), as Connections and SSH Config name files: `connections/production/web.conf`
// rather than the absolute path, which takes four lines on a phone. A file outside that folder keeps its
// absolute path. Windows paths use either separator.
export function relativeConfigPath(rootConfig: string, path: string): string {
  const folder = rootConfig.replace(/[\\/][^\\/]*$/u, "");
  if (folder === rootConfig || folder === "" || !path.startsWith(folder)) return path;
  const rest = path.slice(folder.length);
  return /^[\\/]./u.test(rest) ? rest.slice(1) : path;
}
