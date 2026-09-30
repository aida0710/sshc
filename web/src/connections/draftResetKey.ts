import type { HostIdentity } from "../api/config";
import { identityKey } from "./connectionBrowser";

// draftResetKey は、接続エディタの下書きが何をもとにしているか（どの接続の、ファイルのどの版か）を
// 1つの文字列にする。これが変わったら下書きをやり直し、同じ版を読み直しただけなら下書きを残す。
export function draftResetKey(identity: HostIdentity, fileContents: string): string {
  return `${identityKey(identity)}\u0000${fileContents}`;
}
