import type { CreateConnectionDraft, CreationPrerequisite } from "../connections/CreateConnectionModal";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "../keys/workflow";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import type { FileTarget } from "../explorer/ConfigExplorer";
import type { Section } from "../routing/sectionRoute";
import type { InspectorContent } from "../ui/Inspector";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSettings } from "../api/settings";
import type { PasswordVaultStatus } from "../api/vault";
import type { HostEntry } from "../api/config";


export type Navigation = {
  location: BrowserLocation;
  fileTarget: FileTarget | null;
  onFileTargetHandled: (request: number) => void;
  onNavigate: (section: Section) => void;
  onNavigateLocation: (url: string, options?: NavigateLocationOptions) => void;
  onNavigateForCreation: (section: CreationPrerequisite) => void;
  onOpenFile: (path: string, line: number) => void;
  onNavigationBlockerChange: (blocker: NavigationBlocker | null) => void;
};

export type Handoff = {
  connectionKey: GeneratedPrivateKeyHandoff | null;
  publicKey: GeneratedPublicKeyHandoff | null;
  connectionDraft: CreateConnectionDraft | null;
  onAssignGeneratedKey: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey: (key: GeneratedPublicKeyHandoff) => void;
  onConnectionKeyApplied: () => void;
  onPublicKeyHandled: () => void;
  onConnectionDraftChange: (draft: CreateConnectionDraft | null) => void;
};

export type Shell = {
  onLock: () => void;
  onVaultChanged?: (status: PasswordVaultStatus) => void;
  // passwordless は、Vaultにマスターパスワードが無いかである。
  passwordless: boolean;
  onInspector: (content: InspectorContent) => void;
  terminalSessions: TerminalSessionsState;
  onShowSession: (id: string) => void;
  // onOpenSSHSession は、接続を開いてからそのターミナルを表示する。cwd は開始位置である。
  onOpenSSHSession: (alias: string, cwd?: string) => Promise<void>;
  onOpenWorkspace: (id: string) => void;
  onTerminalSettingsChange: (settings: TerminalSettings) => Promise<void>;
};

export type Declared = {
  groups: string[];
  knownAliases: string[];
  hosts: HostEntry[];
  // hostVPN は、alias ごとに通るVPN経路の名前である。
  hostVPN: Map<string, string>;
};
