import { defineConfig } from "vitepress";

const repository = "https://github.com/aida0710/sshc";
const base = process.env.SSHC_DOCS_BASE ?? "/sshc/";

const jaSidebar = [
  { text: "はじめる", items: [
    { text: "sshcとは", link: "/guide/getting-started" },
    { text: "インストール", link: "/guide/install" },
    { text: "最初の接続", link: "/guide/first-connection" },
  ] },
  { text: "接続を管理する", items: [
    { text: "接続とグループ", link: "/connections/manage" },
    { text: "SSH Config", link: "/connections/config" },
    { text: "認証情報とVault", link: "/connections/credentials" },
    { text: "SSH鍵とKnown Hosts", link: "/connections/keys" },
    { text: "踏み台接続", link: "/connections/proxy" },
  ] },
  { text: "Terminal", items: [
    { text: "Terminalを使う", link: "/features/terminal" },
    { text: "Workspaceと分割", link: "/features/workspace" },
    { text: "クイックコマンド", link: "/terminal/commands" },
    { text: "ポート転送", link: "/terminal/port-forwarding" },
  ] },
  { text: "ファイルと同期", items: [
    { text: "SFTP", link: "/features/sftp" },
    { text: "転送マネージャー", link: "/sftp/transfers" },
    { text: "暗号化同期", link: "/features/sync" },
    { text: "Push・Pull・履歴", link: "/sync/workflow" },
  ] },
  { text: "CLIとプラットフォーム", items: [
    { text: "CLI", link: "/reference/cli" },
    { text: "SerialとTelnet", link: "/cli/serial-telnet" },
    { text: "Android", link: "/platform/android" },
  ] },
  { text: "リファレンス", items: [
    { text: "設定", link: "/reference/settings" },
    { text: "セキュリティ", link: "/reference/security" },
    { text: "トラブルシューティング", link: "/reference/troubleshooting" },
  ] },
];

const enSidebar = [
  { text: "Start here", items: [
    { text: "What is sshc?", link: "/en/guide/getting-started" },
    { text: "Install", link: "/en/guide/install" },
    { text: "First connection", link: "/en/guide/first-connection" },
  ] },
  { text: "Manage connections", items: [
    { text: "Connections and groups", link: "/en/connections/manage" },
    { text: "OpenSSH configuration", link: "/en/connections/config" },
    { text: "Credentials and vault", link: "/en/connections/credentials" },
    { text: "Keys and known hosts", link: "/en/connections/keys" },
    { text: "Jump hosts", link: "/en/connections/proxy" },
  ] },
  { text: "Terminal", items: [
    { text: "Use the terminal", link: "/en/features/terminal" },
    { text: "Workspaces and splits", link: "/en/features/workspace" },
    { text: "Quick Commands", link: "/en/terminal/commands" },
    { text: "Port forwarding", link: "/en/terminal/port-forwarding" },
  ] },
  { text: "Files and sync", items: [
    { text: "SFTP", link: "/en/features/sftp" },
    { text: "Transfer Manager", link: "/en/sftp/transfers" },
    { text: "Encrypted sync", link: "/en/features/sync" },
    { text: "Push, pull, and history", link: "/en/sync/workflow" },
  ] },
  { text: "CLI and platforms", items: [
    { text: "CLI", link: "/en/reference/cli" },
    { text: "Serial and Telnet", link: "/en/cli/serial-telnet" },
    { text: "Android", link: "/en/platform/android" },
  ] },
  { text: "Reference", items: [
    { text: "Settings", link: "/en/reference/settings" },
    { text: "Security", link: "/en/reference/security" },
    { text: "Troubleshooting", link: "/en/reference/troubleshooting" },
  ] },
];

export default defineConfig({
  title: "sshc",
  description: "OpenSSH configuration, terminal, SFTP, workspaces and encrypted sync in one local application.",
  lang: "ja",
  base,
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: "https://aida0710.github.io/sshc/" },
  head: [
    ["meta", { name: "theme-color", content: "#111416" }],
    ["meta", { property: "og:type", content: "website" }],
    ["meta", { property: "og:title", content: "sshc" }],
    ["meta", { property: "og:description", content: "OpenSSH設定の整理、認証情報の再利用、AIエージェント向けCLI、暗号化同期を一つに。" }],
    ["meta", { property: "og:image", content: "https://aida0710.github.io/sshc/images/connections-desktop.png" }],
    ["meta", { name: "twitter:card", content: "summary_large_image" }],
    ["link", { rel: "icon", href: `${base}logo.svg`, type: "image/svg+xml" }],
  ],
  locales: {
    root: {
      label: "日本語",
      lang: "ja",
      themeConfig: {
        nav: [
          { text: "はじめる", link: "/guide/getting-started" },
          { text: "機能", link: "/features/" },
          { text: "リファレンス", link: "/reference/cli" },
          { text: "Releases", link: `${repository}/releases` },
        ],
        sidebar: jaSidebar,
        editLink: { pattern: `${repository}/edit/main/pages/:path`, text: "このページを編集" },
        outlineTitle: "このページの内容",
        lastUpdatedText: "最終更新",
        docFooter: { prev: "前のページ", next: "次のページ" },
      },
    },
    en: {
      label: "English",
      lang: "en",
      link: "/en/",
      description: "Organize OpenSSH, reuse saved credentials from the CLI, and sync securely across devices.",
      head: [
        ["meta", { property: "og:title", content: "sshc" }],
        ["meta", { property: "og:description", content: "Organize OpenSSH, reuse saved credentials from the CLI, and sync securely across devices." }],
      ],
      themeConfig: {
        nav: [
          { text: "Get started", link: "/en/guide/getting-started" },
          { text: "Features", link: "/en/features/" },
          { text: "Reference", link: "/en/reference/cli" },
          { text: "Releases", link: `${repository}/releases` },
        ],
        sidebar: enSidebar,
        editLink: { pattern: `${repository}/edit/main/pages/:path`, text: "Edit this page" },
        outlineTitle: "On this page",
        lastUpdatedText: "Last updated",
        docFooter: { prev: "Previous page", next: "Next page" },
      },
    },
  },
  themeConfig: {
    logo: "/logo.svg",
    siteTitle: "sshc",
    search: { provider: "local" },
    socialLinks: [{ icon: "github", link: repository }],
    footer: {
      message: "Apache License 2.0",
      copyright: "Copyright © sshc contributors",
    },
  },
});
