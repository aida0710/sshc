import { defineDocsConfig } from "@aida0710/app-docs-theme/config";
import { renderMermaidFences } from "./mermaidFence";

const repository = "https://github.com/aida0710/sshc";
const base = process.env.SSHC_DOCS_BASE ?? "/sshc/";

const jaSidebar = [
  { text: "はじめる", items: [
    { text: "sshcとは", link: "/guide/getting-started" },
    { text: "インストール", link: "/guide/install" },
    { text: "最初の接続", link: "/guide/first-connection" },
  ] },
  { text: "Terminal", items: [
    { text: "Terminalを使う", link: "/features/terminal" },
    { text: "ワークスペース", link: "/features/workspace" },
    { text: "分割表示", link: "/features/split-view" },
    { text: "リモートセッション", link: "/features/remote-session" },
    { text: "クイックコマンド", link: "/terminal/commands" },
    { text: "タイトルと通知", link: "/terminal/notifications" },
    { text: "ポート転送", link: "/terminal/port-forwarding" },
  ] },
  { text: "接続を管理する", items: [
    { text: "接続とグループ", link: "/connections/manage" },
    { text: "SSH Config", link: "/connections/config" },
    { text: "認証情報とVault", link: "/connections/credentials" },
    { text: "SSH鍵とKnown Hosts", link: "/connections/keys" },
    { text: "踏み台接続", link: "/connections/proxy" },
  ] },
  { text: "ファイルと同期", items: [
    { text: "SFTP", link: "/features/sftp" },
    { text: "転送マネージャー", link: "/sftp/transfers" },
    { text: "暗号化同期", link: "/features/sync" },
    { text: "Push・Pull・履歴", link: "/sync/workflow" },
    { text: "接続ごとのVPN", link: "/features/vpn" },
  ] },
  { text: "CLIとプラットフォーム", items: [
    { text: "CLI", link: "/reference/cli" },
    { text: "SerialとTelnet", link: "/cli/serial-telnet" },
    { text: "Android", link: "/platform/android" },
  ] },
  { text: "リファレンス", items: [
    { text: "設定", link: "/reference/settings" },
    { text: "エスケープシーケンス", link: "/reference/escape-sequences" },
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
  { text: "Terminal", items: [
    { text: "Use the terminal", link: "/en/features/terminal" },
    { text: "Workspaces", link: "/en/features/workspace" },
    { text: "Split view", link: "/en/features/split-view" },
    { text: "Remote sessions", link: "/en/features/remote-session" },
    { text: "Quick Commands", link: "/en/terminal/commands" },
    { text: "Titles and notifications", link: "/en/terminal/notifications" },
    { text: "Port forwarding", link: "/en/terminal/port-forwarding" },
  ] },
  { text: "Manage connections", items: [
    { text: "Connections and groups", link: "/en/connections/manage" },
    { text: "OpenSSH configuration", link: "/en/connections/config" },
    { text: "Credentials and vault", link: "/en/connections/credentials" },
    { text: "Keys and known hosts", link: "/en/connections/keys" },
    { text: "Jump hosts", link: "/en/connections/proxy" },
  ] },
  { text: "Files and sync", items: [
    { text: "SFTP", link: "/en/features/sftp" },
    { text: "Transfer Manager", link: "/en/sftp/transfers" },
    { text: "Encrypted sync", link: "/en/features/sync" },
    { text: "Push, pull, and history", link: "/en/sync/workflow" },
    { text: "Per-connection VPN", link: "/en/features/vpn" },
  ] },
  { text: "CLI and platforms", items: [
    { text: "CLI", link: "/en/reference/cli" },
    { text: "Serial and Telnet", link: "/en/cli/serial-telnet" },
    { text: "Android", link: "/en/platform/android" },
  ] },
  { text: "Reference", items: [
    { text: "Settings", link: "/en/reference/settings" },
    { text: "Escape sequences", link: "/en/reference/escape-sequences" },
    { text: "Security", link: "/en/reference/security" },
    { text: "Troubleshooting", link: "/en/reference/troubleshooting" },
  ] },
];

const jaDescription = "SSHとローカルシェルを扱うターミナルアプリ。OpenSSH設定をそのまま使い、SFTP、認証情報の再利用、AIエージェント向けCLI、暗号化同期に対応。";
const enDescription = "A terminal app that uses existing OpenSSH configuration, with SFTP, reusable credentials, an AI-friendly CLI, and encrypted sync.";

export default defineDocsConfig({
  name: "sshc",
  description: jaDescription,
  repository,
  base,
  siteUrl: "https://aida0710.github.io/sshc/",
  ogImage: "/images/terminal-desktop.png",
  logo: "/logo.svg",
  favicon: { path: "/logo.svg", type: "image/svg+xml" },
  themeColor: "#111416",
  nav: [
    { text: "はじめる", link: "/guide/getting-started" },
    { text: "機能", link: "/features/" },
    { text: "リファレンス", link: "/reference/cli" },
    { text: "Releases", link: `${repository}/releases` },
  ],
  sidebar: jaSidebar,
  footer: { copyright: "Copyright © www.aida0710.work" },
  english: {
    description: enDescription,
    nav: [
      { text: "Get started", link: "/en/guide/getting-started" },
      { text: "Features", link: "/en/features/" },
      { text: "Reference", link: "/en/reference/cli" },
      { text: "Releases", link: `${repository}/releases` },
    ],
    sidebar: enSidebar,
  },
  markdown: renderMermaidFences,
});
