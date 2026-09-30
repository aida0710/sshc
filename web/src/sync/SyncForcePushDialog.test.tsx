import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Translate } from "../i18n/context";
import { SyncForcePushDialog } from "./SyncForcePushDialog";

const t: Translate = (key) => key;

describe("SyncForcePushDialog", () => {
  it("shows a failed force push inside the dialog with its code", () => {
    render(
      <SyncForcePushDialog
        busy={false}
        error="The snapshot could not be sent."
        errorCode="sync_bucket_unreachable"
        keyConfigured
        message="replace remote"
        t={t}
        onMessageChange={vi.fn()}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    // 警告の文（sync.forceHint）も role="alert" なので、失敗の文から辿る。
    const failure = within(screen.getByRole("dialog")).getByText("The snapshot could not be sent.").closest('[role="alert"]');
    expect(failure).toHaveTextContent("sync_bucket_unreachable");
  });
});
