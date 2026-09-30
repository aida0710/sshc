import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi, type Job } from "./api";

vi.mock("./api", () => ({
  snippetsApi: { library: vi.fn(), preview: vi.fn(), start: vi.fn(), job: vi.fn(), cancel: vi.fn() },
}));

const library = {
  snippets: [
    { id: "alpha", name: "Alpha", command: "uptime", variables: [] },
    { id: "beta", name: "Beta", command: "df -h", variables: [] },
  ],
  startup: [],
} as never;

const preview = {
  snippetId: "alpha",
  evidence: "evidence",
  targets: [
    {
      targetId: "web-1",
      target: { alias: "web-1", hostName: "web-1.example", user: "deploy", port: "22" },
      command: "uptime",
    },
  ],
  actionToken: "token",
  actionExpiresAt: "2026-09-30T00:05:00Z",
} as never;

function runningJob(stdout: string): Job {
  return {
    id: "job-alpha",
    status: "running",
    startedAt: "2026-09-30T00:00:00Z",
    results: [{ targetId: "web-1", alias: "web-1", status: "running", stdout }],
  } as Job;
}

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset().mockResolvedValue(library);
  vi.mocked(snippetsApi.preview).mockReset().mockResolvedValue(preview);
  vi.mocked(snippetsApi.start).mockReset().mockResolvedValue(runningJob("first output"));
  vi.mocked(snippetsApi.job).mockReset();
  vi.mocked(snippetsApi.cancel).mockReset();
});

async function startAlpha(user: ReturnType<typeof userEvent.setup>) {
  const chooser = await screen.findByRole("combobox", { name: "Snippets" });
  await screen.findByRole("option", { name: "Alpha" });
  await user.selectOptions(chooser, "alpha");
  await user.click(screen.getByRole("checkbox", { name: "web-1" }));
  await user.click(screen.getByRole("button", { name: "Preview execution" }));
  await user.click(await screen.findByRole("button", { name: "Run on these hosts" }));
  expect(await screen.findByText("first output")).toBeInTheDocument();
  return chooser;
}

it("keeps the job of a snippet the user has left away when its poll answers late", async () => {
  let answerPoll: (job: Job) => void = () => undefined;
  vi.mocked(snippetsApi.job).mockImplementation(
    () => new Promise<Job>((resolve) => { answerPoll = resolve; }),
  );
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["web-1"]} />);
  const chooser = await startAlpha(user);
  await waitFor(() => expect(snippetsApi.job).toHaveBeenCalledTimes(1), { timeout: 2_000 });

  await user.selectOptions(chooser, "beta");
  expect(screen.queryByText("first output")).not.toBeInTheDocument();
  await act(async () => {
    answerPoll(runningJob("late output"));
  });

  expect(screen.queryByText("late output")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
});

it("says in words that the run had finished when a cancel is refused, instead of dropping the refusal", async () => {
  vi.mocked(snippetsApi.job).mockImplementation(() => new Promise<Job>(() => undefined));
  vi.mocked(snippetsApi.cancel).mockRejectedValue(new ApiError("snippet_job_finished", 409, null));
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["web-1"]} />);
  await startAlpha(user);

  await user.click(screen.getByRole("button", { name: "Cancel" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("The run finished before it could be cancelled.");
  expect(snippetsApi.cancel).toHaveBeenCalledWith("job-alpha");
});
