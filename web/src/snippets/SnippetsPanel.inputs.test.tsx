import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({
  snippetsApi: { library: vi.fn(), preview: vi.fn(), setStartup: vi.fn() },
}));

const library = {
  snippets: [
    {
      id: "tail",
      name: "Tail",
      command: "tail -n {{lines}} app.log",
      variables: [{ name: "lines", type: "integer", required: false, default: "10" }],
    },
  ],
  startup: [],
} as never;

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset().mockResolvedValue(library);
  vi.mocked(snippetsApi.preview).mockReset().mockResolvedValue({ snippetId: "tail", evidence: "", targets: [], actionToken: "", actionExpiresAt: "" });
  vi.mocked(snippetsApi.setStartup).mockReset().mockResolvedValue(undefined);
});

async function typeAndClearLines(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: /Tail/ }));
  const lines = screen.getByRole("spinbutton", { name: "lines · Value" });
  await user.type(lines, "5");
  await user.clear(lines);
}

it("previews with no value for a number typed and then cleared, so the engine uses the default", async () => {
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["r100"]} />);
  await typeAndClearLines(user);

  await user.click(screen.getByRole("checkbox", { name: "r100" }));
  await user.click(screen.getByRole("button", { name: "Preview execution" }));

  expect(snippetsApi.preview).toHaveBeenCalledWith("tail", ["r100"], {});
});

it("saves no value in the startup snippet for a number typed and then cleared", async () => {
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["r100"]} />);
  await typeAndClearLines(user);

  await user.click(screen.getByRole("button", { name: "Set startup snippet" }));

  expect(snippetsApi.setStartup).toHaveBeenCalledWith("r100", "tail", {});
});
