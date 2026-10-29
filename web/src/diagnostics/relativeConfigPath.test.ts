import { describe, expect, it } from "vitest";
import { relativeConfigPath } from "./relativeConfigPath";

describe("relativeConfigPath", () => {
  it("names a file in the root configuration's folder by its path there", () => {
    expect(relativeConfigPath("/home/demo/.ssh/config", "/home/demo/.ssh/connections/production/web.conf")).toBe("connections/production/web.conf");
    expect(relativeConfigPath("/home/demo/.ssh/config", "/home/demo/.ssh/config")).toBe("config");
    expect(relativeConfigPath("C:\\Users\\demo\\.ssh\\config", "C:\\Users\\demo\\.ssh\\conf.d\\lab.conf")).toBe("conf.d\\lab.conf");
  });

  it("keeps the absolute path of a file outside that folder", () => {
    expect(relativeConfigPath("/home/demo/.ssh/config", "/etc/ssh/ssh_config")).toBe("/etc/ssh/ssh_config");
    expect(relativeConfigPath("/home/demo/.ssh/config", "/home/demo/.ssh-old/config")).toBe("/home/demo/.ssh-old/config");
  });
});
