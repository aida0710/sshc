// connectionAddress は、一覧やカードに出す接続先の住所（user@host:port）を作る。
// Home・Connections・SFTP で同じ書き方にする。ポートは SSH の既定の 22 なら書かず、IPv6 は角かっこで囲む。
export function connectionAddress({ hostName, user = "", port = "" }: { hostName: string; user?: string; port?: string }): string {
  const host = hostName.includes(":") && !hostName.startsWith("[") ? `[${hostName}]` : hostName;
  if (host === "") return "";
  const account = user === "" ? "" : `${user}@`;
  const shownPort = port === "" || port === "22" ? "" : `:${port}`;
  return `${account}${host}${shownPort}`;
}
