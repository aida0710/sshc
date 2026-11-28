// sshc の GitHub のリポジトリ。画面のコミットとリリースのページへのリンクは、この下を指す。
export const sshcRepositoryURL = "https://github.com/aida0710/sshc";

// 本物の sshc エンジンが返すリリースのページ（releases/tag/<バージョン>）は、どれもこの下にある。
const releasePagePrefix = `${sshcRepositoryURL}/releases/`;

// 画面は pageUrl をそのまま href に入れ、ブラウザは画面の URL を基準に解釈する。`https:/…` のように
// `//` の無い値は、https で開いた画面では画面と同じオリジンへのリンクになる。`https://` で始まる値は基準によらない。
const absoluteHttpsPrefix = "https://";

// isReleasePageURL は、pageURL が sshc の GitHub のリリースのページかを返す。リモートセッションでは
// pageUrl を接続先のエンジンが返すので、更新の案内のリンクにしてよいかをこれで決める。
// 正規化した href で比べるので、`..`・ユーザー名・ポートで別の場所を指す値もここで外れる。
export function isReleasePageURL(pageURL: string | undefined): pageURL is string {
  if (pageURL === undefined || !pageURL.startsWith(absoluteHttpsPrefix)) return false;
  try {
    return new URL(pageURL).href.startsWith(releasePagePrefix);
  } catch {
    return false;
  }
}
