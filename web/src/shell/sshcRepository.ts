// sshc の GitHub のリポジトリ。画面のコミットとリリースのページへのリンクは、この下を指す。
export const sshcRepositoryURL = "https://github.com/aida0710/sshc";

// 本物の sshc エンジンが返すリリースのページ（releases/tag/<バージョン>）は、どれもこの下にある。
const releasePagePrefix = `${sshcRepositoryURL}/releases/`;

// isReleasePageURL は、pageURL が sshc の GitHub のリリースのページかを返す。リモートセッションでは
// pageUrl を接続先のエンジンが返すので、更新の案内のリンクにしてよいかをこれで決める。
// ブラウザと同じく正規化した href で比べるので、`..`・ユーザー名・ポートで別の場所を指す値もここで外れる。
export function isReleasePageURL(pageURL: string | undefined): pageURL is string {
  if (pageURL === undefined) return false;
  try {
    return new URL(pageURL).href.startsWith(releasePagePrefix);
  } catch {
    return false;
  }
}
