import { describe, expect, it } from "vitest";
import { parseOSC9Directory } from "./osc9Directory";

describe("parseOSC9Directory", () => {
  it("Windowsのドライブのパスを、OpenSSH for WindowsのSFTPの形にする", () => {
    expect(parseOSC9Directory("9;\"C:\\Users\\aida\\src\"")).toBe("/C:/Users/aida/src");
    expect(parseOSC9Directory("9;c:\\")).toBe("/C:/");
    expect(parseOSC9Directory("9;D:")).toBe("/D:/");
  });

  it("Unixのパスはそのまま使う", () => {
    expect(parseOSC9Directory("9;/home/aida")).toBe("/home/aida");
  });

  it("ネットワークのパスや改行を含むものは使わない", () => {
    expect(parseOSC9Directory("9;\\\\server\\share")).toBeNull();
    expect(parseOSC9Directory("9;//server/share")).toBeNull();
    expect(parseOSC9Directory("9;C:\\a\nb")).toBeNull();
    expect(parseOSC9Directory("9;relative\\path")).toBeNull();
  });

  it("フォルダではないOSC 9は扱わない", () => {
    expect(parseOSC9Directory("4;1;50")).toBeNull();
    expect(parseOSC9Directory("build finished")).toBeNull();
  });
});
