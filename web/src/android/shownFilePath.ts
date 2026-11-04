import { runsInAndroidApp } from "./native";

// shownFilePath names a file on screen. In the Android app ~/.ssh is inside the app's private folder, which means
// nothing on a phone, so a file there is named as every page names a file under ~/.ssh.
export function shownFilePath(path: string): string {
  return runsInAndroidApp() ? path.replace(/^.*\/\.ssh\//u, "~/.ssh/") : path;
}
