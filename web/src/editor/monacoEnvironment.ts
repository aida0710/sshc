import editorWorkerURL from "monaco-editor/editor/editor.worker.js?worker&url";
import jsonWorkerURL from "monaco-editor/language/json/json.worker.js?worker&url";
import type { ITrustedTypePolicy, ITrustedTypePolicyOptions } from "monaco-editor/editor/editor.api.js";

// Monaco's name for the policy that turns a worker script's URL into a
// TrustedScriptURL. The engine's Content Security Policy allows it with the
// other policies Monaco creates (internal/httpserver/security.go).
const workerFactoryPolicyName = "defaultWorkerFactory";

// A worker must be started as the kind of script Vite made it. A build makes
// the bundled workers classic scripts, because vite.config.ts leaves
// worker.format at its default ("iife"). The dev server serves them as ES
// modules.
const bundledWorkerType: WorkerType = import.meta.env.DEV ? "module" : "classic";

type TrustedTypePolicyFactory = {
  createPolicy(name: string, options: ITrustedTypePolicyOptions): ITrustedTypePolicy;
};

function browserTrustedTypes(): TrustedTypePolicyFactory | undefined {
  return (globalThis as { trustedTypes?: TrustedTypePolicyFactory }).trustedTypes;
}

// Monaco asks for each of its policies here instead of asking the browser.
// Every policy is created as Monaco asks and is kept only by the Monaco module
// that asked, so no other script can reach it.
//
// The worker policy is the exception: Monaco gets none. Monaco 0.56 asks for
// defaultWorkerFactory from two modules, its editor and the worker client of
// the JSON support, and the engine's policy carries no 'allow-duplicates', so
// the browser would refuse the second creation as a violation. Monaco uses the
// policy only when it starts a worker itself, and getWorker starts every
// worker here.
function createPolicyForMonaco(name: string, options: ITrustedTypePolicyOptions = {}): ITrustedTypePolicy | undefined {
  if (name === workerFactoryPolicyName) return undefined;
  return browserTrustedTypes()?.createPolicy(name, options);
}

// The workers are same-origin Vite assets. Monaco's own fallback starts them
// from a blob: URL, which the engine's script policy does not allow. Where
// Trusted Types are enforced, the Worker constructor accepts only a
// TrustedScriptURL, so the URL goes through the worker policy.
function startBundledWorker(label: string, scriptURLPolicy: ITrustedTypePolicy | undefined): Worker {
  const url = label === "json" ? jsonWorkerURL : editorWorkerURL;
  return new Worker(scriptURLPolicy?.createScriptURL?.(url) ?? url, { type: bundledWorkerType, name: label });
}

// installMonacoEnvironment must run before Monaco is first imported. Monaco
// reads MonacoEnvironment, and creates most of its policies, while its modules
// are evaluated. loadMonacoEditor.ts keeps that order.
export function installMonacoEnvironment(): void {
  if (globalThis.MonacoEnvironment !== undefined) return;
  // Created once, under Monaco's name, and held only by getWorker below.
  const workerScriptURLPolicy = browserTrustedTypes()?.createPolicy(workerFactoryPolicyName, { createScriptURL: (url) => url });
  globalThis.MonacoEnvironment = {
    getWorker: (_workerId, label) => startBundledWorker(label, workerScriptURLPolicy),
    createTrustedTypesPolicy: createPolicyForMonaco,
  };
}
