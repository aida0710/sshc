export const consoleDragMimeType = "application/x-sshc-console";

export type LiveWorkspaceSummary = {
  id: string;
  name: string;
  memberSessionIds: string[];
  focusedSessionId: string;
};
