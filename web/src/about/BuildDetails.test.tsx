import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { About } from "../api/about";
import { BuildDetails } from "./BuildDetails";

const build = { version: "v1.0.0", revision: "", goVersion: "go1.26.10", arch: "arm64", manager: "unknown", executable: "/usr/local/bin/sshc" };

describe("BuildDetails", () => {
  it("says how sshc was installed and where it runs from", () => {
    render(<BuildDetails about={{ ...build, os: "linux" } as About} />);
    expect(screen.getByText("Installed with")).toBeVisible();
    expect(screen.getByText("/usr/local/bin/sshc")).toBeVisible();
  });

  it("leaves both out in the Android app, where neither says anything about sshc", () => {
    render(<BuildDetails about={{ ...build, os: "android", executable: "/system/bin/app_process64" } as About} />);
    expect(screen.queryByText("Installed with")).toBeNull();
    expect(screen.queryByText("/system/bin/app_process64")).toBeNull();
    expect(screen.getByText("android / arm64")).toBeVisible();
  });
});
