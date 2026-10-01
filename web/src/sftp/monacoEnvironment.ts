import editorWorkerURL from "monaco-editor/editor/editor.worker.js?worker&url";
import jsonWorkerURL from "monaco-editor/language/json/json.worker.js?worker&url";
import type { Environment, ITrustedTypePolicy, ITrustedTypePolicyOptions } from "monaco-editor/editor/editor.api.js";

// Monaco's name for the policy that turns a worker script's URL into a
// TrustedScriptURL. The engine's Content Security Policy allows it with the
// other policies Monaco creates (internal/httpserver/security.go).
const workerFactoryPolicyName = "defaultWorkerFactory";

type TrustedTypePolicyFactory = {
  createPolicy(name: string, options: ITrustedTypePolicyOptions): ITrustedTypePolicy;
};

// Monaco 0.56 creates defaultWorkerFactory from two modules: its editor and
// the worker client of the JSON support it loads with a JSON file. The engine's
// policy carries no 'allow-duplicates', so the browser refuses the second
// creation as a violation. Each name is therefore created once, and asking for
// it again returns that policy. Every policy Monaco creates passes its value
// through unchanged, so the second caller gets the policy it asked for.
const createdPolicies = new Map<string, ITrustedTypePolicy | undefined>();

function createTrustedTypesPolicy(name: string, options: ITrustedTypePolicyOptions = {}): ITrustedTypePolicy | undefined {
  if (createdPolicies.has(name)) return createdPolicies.get(name);
  const factory = (globalThis as { trustedTypes?: TrustedTypePolicyFactory }).trustedTypes;
  const policy = factory?.createPolicy(name, options);
  createdPolicies.set(name, policy);
  return policy;
}

// The workers are same-origin Vite assets. Monaco's own fallback starts them
// from a blob: URL, which the engine's script policy does not allow. Where
// Trusted Types are enforced, the Worker constructor accepts only a
// TrustedScriptURL, so the URL goes through Monaco's worker policy.
function createWorker(_workerId: string, label: string): Worker {
  const url = label === "json" ? jsonWorkerURL : editorWorkerURL;
  const policy = createTrustedTypesPolicy(workerFactoryPolicyName, { createScriptURL: (value) => value });
  return new Worker(policy?.createScriptURL?.(url) ?? url, { name: label });
}

const monacoEnvironment: Environment = { getWorker: createWorker, createTrustedTypesPolicy };

// installMonacoEnvironment must run before Monaco is first imported. Monaco
// reads MonacoEnvironment, and creates most of its policies, while its modules
// are evaluated.
export function installMonacoEnvironment(): void {
  globalThis.MonacoEnvironment ??= monacoEnvironment;
}
