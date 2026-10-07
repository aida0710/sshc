export const messages = {
  pageTitle: "sshc · ブラウザデモ",
  demoTitle: "ブラウザデモ",
  reset: "最初からやり直す",
  noInstall: "インストール不要",
  heading: "ブラウザ内で、sshcを試す。",
  introduction: "このブラウザで軽量LinuxのVMを3台起動します。Web UIとCLIから、実際にSSH接続やSFTPを操作できます。",
  machine: "VM",
  purpose: "役割",
  memory: "メモリ",
  clientPurpose: "sshcエンジンとCLI",
  serverPurpose: "SSH・SFTP接続先",
  memoryDetail: (memoryMiB) => `VMのメモリは合計${memoryMiB} MiB。ブラウザ自体のメモリは別途使います。起動すると約40 MBをダウンロードします。操作内容はこのブラウザ内にだけ保持され、ページを閉じると消えます。Web UIのファイル操作は2 MiBまでの小さなファイルでお試しください。`,
  confirm: "この構成で起動します。よろしいですか？",
  start: "起動する",
  webTab: "Web UI",
  cliTab: "CLI",
  tabsLabel: "デモ操作",
  frameTitle: "sshc Web UI",
  commandExamples: "例:",
  loading: "VMイメージを読み込み、ブラウザ内で3台を起動しています…",
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
