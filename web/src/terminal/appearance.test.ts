import { describe, expect, it } from "vitest";
import { resolveAppearance } from "./appearance";

describe("resolveAppearance", () => {
  it("lets the connection win over the overall choice", () => {
    expect(resolveAppearance({ palette: "nord" }, { palette: "dracula" }).palette).toBe("nord");
  });

  it("falls back to the overall choice", () => {
    expect(resolveAppearance({}, { palette: "dracula" }).palette).toBe("dracula");
    expect(resolveAppearance(undefined, { palette: "dracula" }).palette).toBe("dracula");
  });

  // **項目ごとに重ねる。** 接続に配色だけを置いた人の字体が、そこで既定へ
  // 戻ってはならない。
  it("keeps the overall font when the connection only chose a palette", () => {
    const resolved = resolveAppearance({ palette: "nord" }, { palette: "dracula", font: "jetbrains-mono" });
    expect(resolved).toEqual({ palette: "nord", font: "jetbrains-mono" });
  });

  // **空は選択ではない。** 接続で「既定へ戻す」を選んだ人は、全体の選択へ
  // 戻るのであって、全体まで消したいわけではない。
  it("reads an empty choice as no choice, not as a reset of everything", () => {
    expect(resolveAppearance({ palette: "" }, { palette: "dracula" }).palette).toBe("dracula");
  });

  it("chooses nothing when nobody chose", () => {
    expect(resolveAppearance(undefined, undefined)).toEqual({ palette: "", font: "" });
  });
});
