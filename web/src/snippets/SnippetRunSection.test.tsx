import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SnippetRunSection } from "./SnippetRunSection";
import type { Preview } from "./api";

const preview = {
  snippetId: "alpha",
  evidence: "evidence",
  actionToken: "token",
  actionExpiresAt: "2026-10-27T00:05:00Z",
  targets: [{
    targetId: "gpu-01",
    target: {
      alias: "gpu-01",
      hostName: "gpu-01.lab.internal",
      user: "aida",
      port: "22",
      route: [
        { alias: "bastion", hostName: "203.0.113.10", user: "ops", port: "2222" },
        { alias: "relay", hostName: "198.51.100.9", user: "ops", port: "22", proxyCommand: "nc %h %p" },
      ],
    },
    command: "nvidia-smi",
  }],
} as unknown as Preview;

it("names a ProxyCommand only for the hops that have one", () => {
  render(
    <SnippetRunSection
      aliases={["gpu-01"]}
      targets={["gpu-01"]}
      preview={preview}
      job={null}
      previewing={false}
      running={false}
      cancelling={false}
      busy={false}
      onTargetsChange={vi.fn()}
      onPreview={vi.fn()}
      onRun={vi.fn()}
      onCancel={vi.fn()}
    />,
  );

  expect(screen.getByText("1. ops@203.0.113.10:2222")).toBeInTheDocument();
  expect(screen.getByText("2. ops@198.51.100.9:22 · ProxyCommand: nc %h %p")).toBeInTheDocument();
  expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
});
