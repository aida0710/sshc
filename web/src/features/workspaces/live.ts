export const sessionDragMimeType = "application/x-sshc-session";

export type LiveWorkspaceSummary = {
  id: string;
  name: string;
  memberSessionIds: string[];
  focusedSessionId: string;
};
