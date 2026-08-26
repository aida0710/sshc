import { apiClient } from "../api/client";
import { asArray, asBoolean, asNumber, asRecord, asString, jsonHeaders } from "../api/guards";

export type SnippetVariable = { name: string; type: "string" | "integer" | "boolean" | "secret"; required?: boolean; default?: string; description?: string };
export type Snippet = { id: string; name: string; description?: string; command: string; variables: SnippetVariable[]; createdAt: string; updatedAt: string };
export type Startup = { alias: string; snippetId: string; inputs?: Record<string, string> };
export type SnippetDraft = Pick<Snippet, "name" | "command"> & { description: string; variables: SnippetVariable[] };
export type ExecutionTarget = { targetId: string; alias: string };
export type ExecutionPreviewRequest = { snippetId?: string; command?: string; targets: ExecutionTarget[]; inputs: Record<string, string> };
export type RouteHop = { alias: string; hostName: string; user: string; port: string; proxyCommand: string; strictHostKey: string; authentication: string[]; identityFiles: string[]; identitiesOnly: boolean; hostKeyAlgorithms: string[] };
export type Preview = { snippetId: string; evidence: string; actionToken: string; actionExpiresAt: string; targets: { targetId: string; target: { alias: string; hostName: string; user: string; port: string; route: RouteHop[] }; command: string }[] };
export type Job = { id: string; status: "running" | "completed" | "cancelled"; startedAt: string; finishedAt?: string; results: { targetId: string; alias: string; status: string; exitCode?: number; stdout?: string; stderr?: string; truncated?: boolean; problem?: string }[] };

function variable(value: unknown): SnippetVariable {
  const item = asRecord(value);
  const type = asString(item.type);
  if (type !== "string" && type !== "integer" && type !== "boolean" && type !== "secret") throw new Error("invalid_response");
  return { name: asString(item.name), type, ...(item.required === undefined ? {} : { required: asBoolean(item.required) }), ...(item.default === undefined ? {} : { default: asString(item.default) }), ...(item.description === undefined ? {} : { description: asString(item.description) }) };
}

function snippet(value: unknown): Snippet {
  const item = asRecord(value);
  return { id: asString(item.id), name: asString(item.name), command: asString(item.command), variables: asArray(item.variables ?? []).map(variable), createdAt: asString(item.createdAt), updatedAt: asString(item.updatedAt), ...(item.description === undefined ? {} : { description: asString(item.description) }) };
}

function job(value: unknown): Job {
  const item = asRecord(value);
  const status = asString(item.status);
  if (status !== "running" && status !== "completed" && status !== "cancelled") throw new Error("invalid_response");
  return { id: asString(item.id), status, startedAt: asString(item.startedAt), ...(item.finishedAt === undefined ? {} : { finishedAt: asString(item.finishedAt) }), results: asArray(item.results).map((raw) => { const result = asRecord(raw); return { targetId: asString(result.targetId), alias: asString(result.alias), status: asString(result.status), ...(result.exitCode === undefined ? {} : { exitCode: asNumber(result.exitCode) }), ...(result.stdout === undefined ? {} : { stdout: asString(result.stdout) }), ...(result.stderr === undefined ? {} : { stderr: asString(result.stderr) }), ...(result.truncated === undefined ? {} : { truncated: asBoolean(result.truncated) }), ...(result.problem === undefined ? {} : { problem: asString(result.problem) }) }; }) };
}

function parsePreview(value: unknown): Preview {
  const item = asRecord(value);
  return { snippetId: asString(item.snippetId), evidence: asString(item.evidence), actionToken: asString(item.actionToken), actionExpiresAt: asString(item.actionExpiresAt), targets: asArray(item.targets).map((raw) => { const targetPreview = asRecord(raw); const target = asRecord(targetPreview.target); return { targetId: asString(targetPreview.targetId), target: { alias: asString(target.alias), hostName: asString(target.hostName), user: asString(target.user), port: asString(target.port), route: asArray(target.route ?? []).map((rawHop) => { const hop = asRecord(rawHop); return { alias: asString(hop.alias), hostName: asString(hop.hostName), user: asString(hop.user), port: asString(hop.port), proxyCommand: asString(hop.proxyCommand ?? ""), strictHostKey: asString(hop.strictHostKey), authentication: asArray(hop.authentication ?? []).map(asString), identityFiles: asArray(hop.identityFiles ?? []).map(asString), identitiesOnly: hop.identitiesOnly === true, hostKeyAlgorithms: asArray(hop.hostKeyAlgorithms ?? []).map(asString) }; }) }, command: asString(targetPreview.command) }; }) };
}

export const snippetsApi = {
  async library(): Promise<{ snippets: Snippet[]; startup: Startup[] }> {
    const value = asRecord(await apiClient.read("/api/v1/snippets"));
    return { snippets: asArray(value.snippets).map(snippet), startup: asArray(value.startup).map((raw) => { const item = asRecord(raw); return { alias: asString(item.alias), snippetId: asString(item.snippetId), ...(item.inputs === undefined ? {} : { inputs: Object.fromEntries(Object.entries(asRecord(item.inputs)).map(([key, input]) => [key, asString(input)])) }) }; }) };
  },
  async create(draft: SnippetDraft): Promise<Snippet> { return snippet(await apiClient.mutate<unknown>("/api/v1/snippets", { method: "POST", headers: jsonHeaders, body: JSON.stringify(draft) })); },
  async update(id: string, draft: SnippetDraft): Promise<Snippet> { return snippet(await apiClient.mutate<unknown>(`/api/v1/snippets/${encodeURIComponent(id)}`, { method: "PUT", headers: jsonHeaders, body: JSON.stringify(draft) })); },
  async remove(id: string): Promise<void> { const response = await apiClient.send(`/api/v1/snippets/${encodeURIComponent(id)}`, { method: "DELETE" }); if (!response.ok) throw new Error("snippet_delete_failed"); },
  async setStartup(alias: string, snippetId: string, inputs: Record<string, string>): Promise<void> { await apiClient.mutate(`/api/v1/snippets/startup/${encodeURIComponent(alias)}`, { method: "PUT", headers: jsonHeaders, body: JSON.stringify({ snippetId, inputs }) }); },
  async preview(snippetId: string, aliases: string[], inputs: Record<string, string>): Promise<Preview> { return parsePreview(await apiClient.mutate<unknown>("/api/v1/snippets/preview", { method: "POST", headers: jsonHeaders, body: JSON.stringify({ snippetId, aliases, inputs }) })); },
  async start(preview: Preview, aliases: string[], inputs: Record<string, string>, concurrency = 4): Promise<Job> { return job(await apiClient.mutate<unknown>("/api/v1/snippets/jobs", { method: "POST", headers: { ...jsonHeaders, "X-SSHC-Action": preview.actionToken }, body: JSON.stringify({ snippetId: preview.snippetId, aliases, inputs, evidence: preview.evidence, concurrency }) })); },
  async previewExecution(request: ExecutionPreviewRequest): Promise<Preview> { return parsePreview(await apiClient.mutate<unknown>("/api/v1/snippets/preview", { method: "POST", headers: jsonHeaders, body: JSON.stringify(request) })); },
  async startExecution(preview: Preview, request: ExecutionPreviewRequest, concurrency = 4): Promise<Job> { return job(await apiClient.mutate<unknown>("/api/v1/snippets/jobs", { method: "POST", headers: { ...jsonHeaders, "X-SSHC-Action": preview.actionToken }, body: JSON.stringify({ ...request, evidence: preview.evidence, concurrency }) })); },
  async job(id: string): Promise<Job> { return job(await apiClient.read(`/api/v1/snippets/jobs/${encodeURIComponent(id)}`)); },
  async cancel(id: string): Promise<Job> { return job(await apiClient.mutate<unknown>(`/api/v1/snippets/jobs/${encodeURIComponent(id)}`, { method: "DELETE" })); },
};
