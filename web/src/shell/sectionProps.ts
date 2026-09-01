import type { CreateConnectionDraft, CreationPrerequisite } from "../connections/CreateConnectionModal";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "../keys/workflow";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import type { FileTarget } from "../explorer/ConfigExplorer";
import type { Section } from "../routing/sectionRoute";
import type { InspectorContent } from "../ui/Inspector";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSettings } from "../api/integrations";
import type { HostEntry } from "../api/config";


export type Navigation = {
  location: BrowserLocation;
  fileTarget: FileTarget | null;
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
  onInspector: (content: InspectorContent) => void;
  consoles: TerminalSessionsState;
  onShowConsole: (id: string) => void;
  onOpenWorkspace: (id: string) => void;
  onTerminalSettingsChange: (settings: TerminalSettings) => Promise<void>;
};

export type Declared = {
  groups: string[];
  knownAliases: string[];
  hosts: HostEntry[];
};
