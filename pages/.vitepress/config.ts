import { defineConfig } from "vitepress";

const repository = "https://github.com/aida0710/sshc";
const base = process.env.SSHC_DOCS_BASE ?? "/sshc/";

const jaSidebar = {
  "/guide/": [{ text: "はじめる", items: [
    { text: "概要", link: "/guide/getting-started" },
    { text: "インストール", link: "/guide/install" },
  ] }],
  "/features/": [{ text: "機能", items: [
    { text: "機能一覧", link: "/features/" },
    { text: "接続と Terminal", link: "/features/terminal" },
    { text: "SFTP", link: "/features/sftp" },
    { text: "Workspace", link: "/features/workspace" },
    { text: "同期", link: "/features/sync" },
  ] }],
  "/reference/": [{ text: "リファレンス", items: [
    { text: "CLI", link: "/reference/cli" },
    { text: "トラブルシューティング", link: "/reference/troubleshooting" },
    { text: "セキュリティ", link: "/reference/security" },
  ] }],
};

const enSidebar = {
  "/en/guide/": [{ text: "Get started", items: [
    { text: "Overview", link: "/en/guide/getting-started" },
    { text: "Installation", link: "/en/guide/install" },
  ] }],
  "/en/features/": [{ text: "Features", items: [
    { text: "Feature overview", link: "/en/features/" },
    { text: "Connections and terminal", link: "/en/features/terminal" },
    { text: "SFTP", link: "/en/features/sftp" },
    { text: "Workspaces", link: "/en/features/workspace" },
    { text: "Sync", link: "/en/features/sync" },
  ] }],
  "/en/reference/": [{ text: "Reference", items: [
    { text: "CLI", link: "/en/reference/cli" },
    { text: "Troubleshooting", link: "/en/reference/troubleshooting" },
    { text: "Security", link: "/en/reference/security" },
  ] }],
};

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
    ["meta", { property: "og:title", content: "sshc — OpenSSHを、そのまま管理する" }],
    ["meta", { property: "og:description", content: "接続、Terminal、SFTP、Workspace、暗号化同期を一つのローカルアプリケーションで。" }],
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
      description: "Connections, terminals, SFTP, workspaces and encrypted sync in one local application.",
      head: [
        ["meta", { property: "og:title", content: "sshc — Manage OpenSSH without replacing it" }],
        ["meta", { property: "og:description", content: "Connections, terminals, SFTP, workspaces and encrypted sync in one local application." }],
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
