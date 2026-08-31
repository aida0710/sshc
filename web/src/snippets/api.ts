import { apiClient } from "../api/client";
import { jsonHeaders } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

export type SnippetVariable = components["schemas"]["SnippetVariable"];
export type Snippet = components["schemas"]["Snippet"];
export type Startup = components["schemas"]["StartupSnippet"];
export type SnippetDraft = components["schemas"]["SnippetDraft"];
export type Preview = components["schemas"]["SnippetPreview"];
export type Job = components["schemas"]["SnippetJob"];
export type RouteHop = NonNullable<Preview["targets"][number]["target"]["route"]>[number];

function snippet(value: unknown): Snippet {
  return validateOpenAPISchema<Snippet>("Snippet", value);
}

function job(value: unknown): Job {
  return validateOpenAPISchema<Job>("SnippetJob", value);
}

function parsePreview(value: unknown): Preview {
  return validateOpenAPISchema<Preview>("SnippetPreview", value);
}

export const snippetsApi = {
  async library(): Promise<{ snippets: Snippet[]; startup: Startup[] }> {
    return validateOpenAPISchema<components["schemas"]["SnippetLibrary"]>("SnippetLibrary", await apiClient.read("/api/v1/snippets"));
  },
  async create(draft: SnippetDraft): Promise<Snippet> { return snippet(await apiClient.mutate<unknown>("/api/v1/snippets", { method: "POST", headers: jsonHeaders, body: JSON.stringify(draft) })); },
  async update(id: string, draft: SnippetDraft): Promise<Snippet> { return snippet(await apiClient.mutate<unknown>(`/api/v1/snippets/${encodeURIComponent(id)}`, { method: "PUT", headers: jsonHeaders, body: JSON.stringify(draft) })); },
  async remove(id: string): Promise<void> { const response = await apiClient.send(`/api/v1/snippets/${encodeURIComponent(id)}`, { method: "DELETE" }); if (!response.ok) throw new Error("snippet_delete_failed"); },
  async setStartup(alias: string, snippetId: string, inputs: Record<string, string>): Promise<void> { await apiClient.mutate(`/api/v1/snippets/startup/${encodeURIComponent(alias)}`, { method: "PUT", headers: jsonHeaders, body: JSON.stringify({ snippetId, inputs }) }); },
  async preview(snippetId: string, aliases: string[], inputs: Record<string, string>): Promise<Preview> { return parsePreview(await apiClient.mutate<unknown>("/api/v1/snippets/preview", { method: "POST", headers: jsonHeaders, body: JSON.stringify({ snippetId, aliases, inputs }) })); },
  async start(preview: Preview, aliases: string[], inputs: Record<string, string>, concurrency = 4): Promise<Job> { return job(await apiClient.mutate<unknown>("/api/v1/snippets/jobs", { method: "POST", headers: { ...jsonHeaders, "X-SSHC-Action": preview.actionToken }, body: JSON.stringify({ snippetId: preview.snippetId, aliases, inputs, evidence: preview.evidence, concurrency }) })); },
  async job(id: string): Promise<Job> { return job(await apiClient.read(`/api/v1/snippets/jobs/${encodeURIComponent(id)}`)); },
  async cancel(id: string): Promise<Job> { return job(await apiClient.mutate<unknown>(`/api/v1/snippets/jobs/${encodeURIComponent(id)}`, { method: "DELETE" })); },
};
