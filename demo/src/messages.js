export const messages = {
  pageTitle: "sshc · ブラウザデモ",
  demoTitle: "ブラウザデモ",
  updatedAtLabel: "更新日時:",
  versionLabel: "バージョン:",
  checkingRelease: "GitHubの最新リリースを確認しています…",
  latestRelease: (version) => `起動する版: ${version}（GitHub最新リリース）`,
  releaseFallback: (version) => `最新リリースを取得できないため、配信済みの${version}で起動します。`,
  releaseArchiveProgress: ({ state, fraction, completedFiles, totalFiles }) => state === "downloading"
    ? `デモ一式をダウンロード中 · ${Math.round(fraction * 100)}%`
    : state === "unpacking" ? "デモ一式を展開しています…"
    : state === "caching" ? `デモ一式を保存中 · ${completedFiles}/${totalFiles}` : "デモ一式の準備が完了しました",
  clearUICache: "キャッシュを削除して再読み込み",
  cacheClearFailed: "再読み込みに失敗しました。もう一度お試しください。",
  updatedAt: (timestamp) => new Intl.DateTimeFormat("ja-JP", {
    timeZone: "Asia/Tokyo", year: "numeric", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", hourCycle: "h23",
  }).format(new Date(timestamp)) + " JST",
  reset: "最初からやり直す",
  noInstall: "インストール不要",
  heading: "ブラウザ内で、sshcを試す。",
  introduction: "このブラウザで軽量LinuxのVMを3台起動します。Web UIとCLIから、実際にSSH接続やSFTPを操作できます。",
  machine: "VM",
  purpose: "役割",
  memory: "メモリ",
  clientPurpose: "sshcエンジンとCLI",
  serverPurpose: "SSH・SFTP接続先",
  memoryDetail: (memoryMiB) => `VMのメモリは合計${memoryMiB} MiB。ブラウザ自体のメモリは別途使います。初回は約45 MBをダウンロードします。操作内容はこのブラウザ内にだけ保持され、ページを閉じると消えます。Web UIのファイル操作は2 MiBまでの小さなファイルでお試しください。`,
  confirm: "この構成で起動します。よろしいですか？",
  start: "起動する",
  webTab: "Web UI",
  cliTab: "CLI",
  tabsLabel: "デモ操作",
  frameTitle: "sshc Web UI",
  commandExamples: "例:",
  startupTitle: "デモを起動しています",
  startupStages: ["読み込み", "VM起動", "sshcエンジン", "Web UI"],
  elapsed: (seconds) => `経過 ${seconds}秒`,
  downloadProgress: (percentage) => percentage === null ? "起動ファイルを読み込んでいます" : `起動ファイルの読み込み ${percentage}%`,
  machineLoading: (percentage) => percentage === null ? "読み込み中" : `読み込み中 · ${percentage}%`,
  machineDownload: (label) => `${label}の起動ファイルの読み込み`,
  uiArchiveProgress: ({ state, downloadPercentage, completedFiles, totalFiles }) => {
    const detail = state === "downloading" ? `ダウンロード中 · ${downloadPercentage}%`
      : state === "unpacking" ? "展開中"
      : state === "caching" ? `準備中 · ${completedFiles}/${totalFiles}`
      : state === "ready" ? "読み込み済み" : "待機中";
    return `Web UIファイル — ${detail}`;
  },
  machineStates: { waiting: "待機中", booting: "Linuxを起動中", engine: "sshcエンジンを起動中", ready: "起動済み", failed: "読み込みに失敗" },
  startupPhases: {
    configuration: "起動の準備をしています…",
    download: "起動ファイルを読み込んでいます…",
    engine: "sshcエンジンを起動しています…",
    web: "Web UIに接続しています…",
  },
  bootProgress: (count) => `VMを起動しています（${count}/3）…`,
  ready: "Web UIとCLIを操作できます。接続先はdemo-aとdemo-bです。",
  failed: "起動に失敗しました。［最初からやり直す］で再度お試しください。",
};

export function applyMessages(document) {
  for (const element of document.querySelectorAll("[data-message]")) {
    element.textContent = messages[element.dataset.message];
  }
  document.title = messages.pageTitle;
  document.querySelector("nav").setAttribute("aria-label", messages.tabsLabel);
  document.getElementById("sshc-ui").title = messages.frameTitle;
}
