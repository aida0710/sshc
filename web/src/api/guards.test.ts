import { describe, expect, it } from "vitest";
import { asArray, asBoolean, asNumber, asRecord, asString, toProblem } from "./guards";
import { ApiError } from "./client";

describe("応答の型検証", () => {
  it("record として通すのはオブジェクトだけ", () => {
    expect(asRecord({ a: 1 })).toEqual({ a: 1 });
    for (const value of [null, [], "x", 1, undefined]) {
      expect(() => asRecord(value)).toThrow("invalid_response");
    }
  });

  it("各フィールドの型を明示的に検証する", () => {
    expect(asArray([1])).toEqual([1]);
    expect(asString("x")).toBe("x");
    expect(asNumber(1)).toBe(1);
    expect(asBoolean(false)).toBe(false);
    expect(() => asArray({})).toThrow();
    expect(() => asString(1)).toThrow();
    expect(() => asNumber("1")).toThrow();
    expect(asNumber(0)).toBe(0);
  });
});

describe("失敗の理由", () => {
  it("サーバーが理由を付けたなら、それを渡す", () => {
    const problem = { code: "vault_locked", message: "locked" };
    expect(toProblem(new ApiError("vault_locked", 409, problem))).toBe(problem);
  });

  it("理由が無い拒否は、code だけを運ぶ", () => {
    expect(toProblem(new ApiError("invalid_request", 400, null)).code).toBe("invalid_request");
  });

  it("API の外で落ちたものも理由になる", () => {
    expect(toProblem(new TypeError("network")).code).toBe("request_failed");
  });
});

describe("型検証関数の配置", () => {
  it("定義はこのファイルにしかない", async () => {
    const { readdir, readFile } = await import("node:fs/promises");
    const { dirname, join } = await import("node:path");
    const { fileURLToPath } = await import("node:url");
    const sourceRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

    async function sources(directory: string): Promise<string[]> {
      const entries = await readdir(directory, { withFileTypes: true });
      const found: string[] = [];
      for (const entry of entries) {
        const path = join(directory, entry.name);
        if (entry.isDirectory()) found.push(...(await sources(path)));
        else if (/\.tsx?$/.test(entry.name) && !entry.name.includes(".test.")) found.push(path);
      }
      return found;
    }

    const guarded = ["asRecord", "asArray", "asString", "asNumber", "asBoolean", "toProblem", "issueAction"];
    const elsewhere: string[] = [];
    for (const path of await sources(sourceRoot)) {
      if (path.endsWith(join("api", "guards.ts"))) continue;
      const body = await readFile(path, "utf8");
      for (const name of guarded) {
        if (new RegExp(`^(?:export )?(?:async )?(?:function|const) ${name}\\b`, "m").test(body)) {
          elsewhere.push(`${path}: ${name}`);
        }
      }
    }
    expect(elsewhere).toEqual([]);
  });
});
