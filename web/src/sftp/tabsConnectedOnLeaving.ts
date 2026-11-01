import type { SFTPPane } from "./sftpPanes";

// The SFTP section's tabs are taken down when another section is opened, and built again from the stored panes, with
// new ids, on coming back. Tabs restored that way wait for Connect, so that opening the app connects nowhere by itself.
// A tab the person had connected when they left the section is different: they were using it a moment ago, and it
// reopens connected. What is kept here is where those tabs were (the pane, the tab and its host) for as long as the
// page lives; a reload starts with none.
let connectedPlaces = new Set<string>();

function place(paneIndex: number, tabIndex: number, alias: string): string {
  return `${paneIndex}:${tabIndex}:${alias}`;
}

// rememberTabsConnectedOnLeaving is called as the section is taken down, with its panes and the ids of the tabs that
// held a connection then.
export function rememberTabsConnectedOnLeaving(panes: readonly SFTPPane[], connectedTabIds: ReadonlySet<string>): void {
  connectedPlaces = new Set(panes.flatMap((pane, paneIndex) => pane.tabs.flatMap((tab, tabIndex) =>
    connectedTabIds.has(tab.id) ? [place(paneIndex, tabIndex, tab.alias)] : [])));
}

// wasConnectedOnLeaving says whether the tab now at this place, with this host, held a connection when the section
// was last left.
export function wasConnectedOnLeaving(paneIndex: number, tabIndex: number, alias: string): boolean {
  return connectedPlaces.has(place(paneIndex, tabIndex, alias));
}
