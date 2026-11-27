import { apiClient } from "../api/client";
import { localApiBase } from "../api/remoteEngines";
import { postJSON, putJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

export type SnippetVariable = components["schemas"]["SnippetVariable"];
export type Snippet = components["schemas"]["Snippet"];
export type Startup = components["schemas"]["StartupSnippet"];
export type SnippetDraft = components["schemas"]["SnippetDraft"];
export type SnippetStep = components["schemas"]["SnippetStep"];
export type Preview = components["schemas"]["SnippetPreview"];
export type Job = components["schemas"]["SnippetJob"];

function snippet(value: unknown): Snippet {
  return validateOpenAPISchema<Snippet>("Snippet", value);
}

function job(value: unknown): Job {
  return validateOpenAPISchema<Job>("SnippetJob", value);
}

function parsePreview(value: unknown): Preview {
  return validateOpenAPISchema<Preview>("SnippetPreview", value);
}

// createSnippetsApi は、base（engineApiBase）のエンジンのスニペットの API である。スニペットを流すのも、そのエンジンが
// その接続先へ行う。
export function createSnippetsApi(base: string) {
  return {
    async library(): Promise<{ snippets: Snippet[]; startup: Startup[] }> {
      return validateOpenAPISchema<components["schemas"]["SnippetLibrary"]>("SnippetLibrary", await apiClient.read(`${base}/snippets`));
    },
    async create(draft: SnippetDraft): Promise<Snippet> { return snippet(await postJSON<unknown>(`${base}/snippets`, draft)); },
    async update(id: string, draft: SnippetDraft): Promise<Snippet> { return snippet(await putJSON<unknown>(`${base}/snippets/${encodeURIComponent(id)}`, draft)); },
    async remove(id: string): Promise<void> { await apiClient.mutate<unknown>(`${base}/snippets/${encodeURIComponent(id)}`, { method: "DELETE" }); },
    async setStartup(alias: string, snippetId: string, inputs: Record<string, string>): Promise<void> { await putJSON(`${base}/snippets/startup/${encodeURIComponent(alias)}`, { snippetId, inputs }); },
    async preview(snippetId: string, aliases: string[], inputs: Record<string, string>): Promise<Preview> { return parsePreview(await postJSON<unknown>(`${base}/snippets/preview`, { snippetId, aliases, inputs })); },
    async start(preview: Preview, aliases: string[], inputs: Record<string, string>, concurrency = 4): Promise<Job> { return job(await postJSON<unknown>(`${base}/snippets/jobs`, { snippetId: preview.snippetId, aliases, inputs, evidence: preview.evidence, concurrency }, preview.actionToken)); },
    async job(id: string): Promise<Job> { return job(await apiClient.read(`${base}/snippets/jobs/${encodeURIComponent(id)}`)); },
    async cancel(id: string): Promise<Job> { return job(await apiClient.mutate<unknown>(`${base}/snippets/jobs/${encodeURIComponent(id)}`, { method: "DELETE" })); },
  };
}

export type SnippetsApi = ReturnType<typeof createSnippetsApi>;

export const snippetsApi = createSnippetsApi(localApiBase);
