import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const oxlint = resolve("node_modules/.bin/oxlint");
const plugin = resolve("lint/idSpelling.mjs");

// 本物の Oxlint にこの規則だけを入れて、咎めた名前を返す。
function flaggedNames(source) {
  const directory = mkdtempSync(join(tmpdir(), "sshc-id-spelling-"));
  try {
    const config = join(directory, ".oxlintrc.json");
    const sample = join(directory, "sample.ts");
    writeFileSync(config, JSON.stringify({ jsPlugins: [plugin], categories: { correctness: "off" }, rules: { "sshc/id-spelling": "error" } }));
    writeFileSync(sample, source);
    const run = spawnSync(oxlint, ["-c", config, "-f", "json", sample], { encoding: "utf8" });
    return JSON.parse(run.stdout).diagnostics.map((diagnostic) => diagnostic.message.match(/^`(\w+)`/)[1]);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

describe("sshc/id-spelling", () => {
  it("型・プロパティ・引数・クラス・enum まで、名前を決める所の `ID` を1回ずつ咎める", () => {
    const names = flaggedNames(`
      export type HostID = string;
      export interface Row { rowID: string }
      export const literal = { tabID: 1 };
      export function takeParam(paneID: number): number { return paneID; }
      export function makeID(): number { return 1; }
      export class Box { boxID = 1; }
      export const { tabID: localID } = literal;
      export enum Kind { kindID = 1 }
    `);
    expect(names).toEqual(["HostID", "rowID", "tabID", "paneID", "makeID", "boxID", "localID", "kindID"]);
  });

  it("他人が決めた名前を読むだけの所と、`Id` や `UUID` の綴りは咎めない", () => {
    const names = flaggedNames(`
      declare const external: { readonly remoteId: number };
      export const viaExternal = (external as unknown as { remoteID: number }).remoteID;
      export const sessionId = 1;
      export const UUID = 2;
    `);
    expect(names).toEqual(["remoteID"]);
  });
});
