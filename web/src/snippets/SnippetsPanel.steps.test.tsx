import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi, type Snippet } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn(), create: vi.fn(), update: vi.fn(), preview: vi.fn() } }));

const savedAt = { createdAt: "2026-10-01T00:00:00Z", updatedAt: "2026-10-01T00:00:00Z" };
const enable: Snippet = {
  id: "enable", name: "Privileged mode", description: "", command: "",
  steps: [
    { wait: "router>", send: "enable", enter: true },
    { wait: "Password:", send: "{{enable_password}}", enter: true },
  ],
  waitSeconds: 20,
  variables: [{ name: "enable_password", type: "secret", required: true }],
  ...savedAt,
};

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset();
  vi.mocked(snippetsApi.create).mockReset();
  vi.mocked(snippetsApi.update).mockReset();
});

it("saves a snippet of steps, each waiting for its text before it types", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [], startup: [] });
  vi.mocked(snippetsApi.create).mockResolvedValue(enable);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["router"]} />);
  await user.click((await screen.findAllByRole("button", { name: "Add snippet" }))[0]!);

  await user.type(screen.getByRole("textbox", { name: "Name" }), "Privileged mode");
  await user.click(screen.getByRole("button", { name: "Steps" }));
  await user.type(screen.getByRole("textbox", { name: "Step 1: wait for" }), "router>");
  await user.type(screen.getByRole("textbox", { name: "Step 1: send" }), "enable");
  await user.click(screen.getByRole("button", { name: "Add step" }));
  await user.type(screen.getByRole("textbox", { name: "Step 2: wait for" }), "Password:");
  await user.type(screen.getByRole("textbox", { name: "Step 2: send" }), "{{{{enable_password}}");
  const waitAtMost = screen.getByRole("spinbutton", { name: "Wait at most" });
  await user.clear(waitAtMost);
  await user.type(waitAtMost, "400");
  expect(screen.getByText("Enter a number of seconds from 1 to 300.")).toBeInTheDocument();
  await user.clear(waitAtMost);
  await user.type(waitAtMost, "20");
  // The variable a step sends is offered like a command's.
  await user.selectOptions(screen.getByRole("combobox", { name: "enable_password · Variable type" }), "secret");
  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(snippetsApi.create).toHaveBeenCalledWith({
    name: "Privileged mode",
    description: "",
    command: "",
    steps: [
      { wait: "router>", send: "enable", enter: true },
      { wait: "Password:", send: "{{enable_password}}", enter: true },
    ],
    waitSeconds: 20,
    variables: [{ name: "enable_password", type: "secret", required: true }],
  });
});

it("carries a command over as the first step, and does not save a step that types nothing", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [], startup: [] });
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["router"]} />);
  await user.click((await screen.findAllByRole("button", { name: "Add snippet" }))[0]!);
  await user.type(screen.getByRole("textbox", { name: "Name" }), "Uptime");
  await user.type(screen.getByRole("textbox", { name: "Command" }), "uptime");

  await user.click(screen.getByRole("button", { name: "Steps" }));
  expect(screen.getByRole("textbox", { name: "Step 1: send" })).toHaveValue("uptime");
  await user.click(screen.getByRole("checkbox", { name: "Press Enter after step 1" }));
  await user.clear(screen.getByRole("textbox", { name: "Step 1: send" }));
  expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
});

it("offers a snippet of steps for connection startup only, not to run on hosts", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [enable], startup: [] });
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["router"]} />);
  const row = await screen.findByRole("button", { name: /Privileged mode/ });
  expect(within(row).getByText("2 steps")).toBeInTheDocument();
  await user.click(row);

  expect(screen.getByRole("textbox", { name: "Step 2: wait for" })).toHaveValue("Password:");
  expect(screen.queryByRole("button", { name: "Preview execution" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Set startup snippet" })).toBeInTheDocument();
});
