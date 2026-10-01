import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import { en } from "../i18n/messages";
import type { MessageKey } from "../i18n/messages";
import { sftpProblemCode, sftpProblemText, sftpTransferProblemText } from "./sftpProblemText";

const t = (key: MessageKey) => en[key];

describe("sftpProblemText", () => {
  it("says an engine code in words", () => {
    expect(sftpProblemText(t, new ApiError("sftp_permission_denied", 403, null))).toBe(en["sftp.problem.permissionDenied"]);
  });

  it("says a code the screen's own transfer threw in words", () => {
    expect(sftpProblemText(t, new Error("sftp_transfer_limit"))).toBe(en["sftp.problem.transferLimit"]);
  });

  it("does not pass a browser's English error sentence through to the screen", () => {
    expect(sftpProblemText(t, new TypeError("Failed to fetch"))).toBe(en["sftp.problem.failed"]);
    expect(sftpProblemText(t, new TypeError("Failed to fetch"), "sftp.problem.deleteFailed")).toBe(en["sftp.problem.deleteFailed"]);
  });
});

describe("the transfer list's problem", () => {
  it("is recorded as a code, and an unknown error as the general failure", () => {
    expect(sftpProblemCode(new ApiError("sftp_conflict", 409, null))).toBe("sftp_conflict");
    expect(sftpProblemCode(new TypeError("Failed to fetch"))).toBe("");
  });

  it("says a name collision and a full or unusable download spool in words", () => {
    expect(sftpTransferProblemText(t, "sftp_name_collision")).toBe(en["sftp.problem.nameCollision"]);
    expect(sftpProblemText(t, new ApiError("sftp_spool_full", 507, null))).toBe(en["sftp.problem.spoolFull"]);
    expect(sftpProblemText(t, new ApiError("sftp_spool_unavailable", 503, null))).toBe(en["sftp.problem.spoolUnavailable"]);
  });

  it("says a folder copied or moved into itself in its own words", () => {
    expect(sftpTransferProblemText(t, "sftp_target_inside_source")).toBe(en["sftp.problem.targetInsideSource"]);
  });

  it("is said in words, including a VPN refusal kept as a code", () => {
    expect(sftpTransferProblemText(t, "transfer_interrupted")).toBe(en["sftp.problem.transferInterrupted"]);
    expect(sftpTransferProblemText(t, "vpn_docker_missing")).toBe(en["vpn.dockerMissing"]);
    expect(sftpTransferProblemText(t, "Failed to fetch")).toBe(en["sftp.problem.failed"]);
  });
});
