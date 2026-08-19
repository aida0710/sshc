// 生成物である。手で編集しない。
//
// 出どころは internal/validate で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。
//
// **パターンは Go の RE2 と JavaScript が同じ意味で読む書き方に限ってある。**
// 文字クラス・アンカー・回数指定だけで、後方参照も先読みも無い。

export const groupSegmentPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
export const aliasPattern = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
export const hostnamePattern = /^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export const maxGroupSegments = 6;
export const maxGroupSegmentBytes = 64;
export const maxAliasLength = 64;
export const maxHostnameLength = 255;

// **大小文字を区別しない。** 既定の macOS ボリュームは "Config" と
// "config" を同じディレクトリエントリとして扱う。
export const reservedGroupNames: ReadonlySet<string> = new Set([
  "authorized_keys",
  "authorized_keys2",
  "config",
  "connections",
  "environment",
  "keys",
  "known_hosts",
  "known_hosts2",
  "rc",
  "sshc",
]);
