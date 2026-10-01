import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { Environment, ITrustedTypePolicyOptions } from "monaco-editor/editor/editor.api.js";

// A trusted value as the fake browser below makes it: the value a policy let
// through, with the name of that policy.
type TrustedValue = { policy: string; value: string };

// A stand-in for the browser's Trusted Types factory that records the name of
// every policy it is asked to create.
function fakeTrustedTypes() {
  const createdNames: string[] = [];
  return {
    createdNames,
    createPolicy(name: string, options: ITrustedTypePolicyOptions) {
      createdNames.push(name);
      return {
        name,
        createScriptURL: (value: string): TrustedValue => ({ policy: name, value: options.createScriptURL?.(value) ?? "" }),
      };
    },
  };
}

// A stand-in for Worker that records how each worker was started.
class RecordedWorker {
  static started: { scriptURL: unknown; options: unknown }[] = [];
  constructor(scriptURL: unknown, options: unknown) {
    RecordedWorker.started.push({ scriptURL, options });
  }
}

// Installs the environment from a fresh copy of the module, so that the
// policies one test creates do not carry over into the next.
async function installedEnvironment(): Promise<Environment> {
  vi.resetModules();
  const { installMonacoEnvironment } = await import("./monacoEnvironment");
  installMonacoEnvironment();
  if (globalThis.MonacoEnvironment === undefined) throw new Error("no MonacoEnvironment was installed");
  return globalThis.MonacoEnvironment;
}

beforeEach(() => {
  globalThis.MonacoEnvironment = undefined;
  RecordedWorker.started = [];
  vi.stubGlobal("Worker", RecordedWorker);
});

afterEach(() => {
  vi.unstubAllGlobals();
  globalThis.MonacoEnvironment = undefined;
});

test("asking for a policy name again returns the policy created first, and the browser creates it only once", async () => {
  const trustedTypes = fakeTrustedTypes();
  vi.stubGlobal("trustedTypes", trustedTypes);
  const environment = await installedEnvironment();

  const first = environment.createTrustedTypesPolicy?.("defaultWorkerFactory", { createScriptURL: (value) => value });
  const second = environment.createTrustedTypesPolicy?.("defaultWorkerFactory", { createScriptURL: (value) => value });

  expect(first).toBeDefined();
  expect(second).toBe(first);
  expect(trustedTypes.createdNames).toEqual(["defaultWorkerFactory"]);
});

test("a worker starts from its bundled script URL made trusted by Monaco's worker policy", async () => {
  const trustedTypes = fakeTrustedTypes();
  vi.stubGlobal("trustedTypes", trustedTypes);
  const environment = await installedEnvironment();

  await environment.getWorker?.("workerMain.js", "json");
  await environment.getWorker?.("workerMain.js", "editorWorkerService");

  expect(trustedTypes.createdNames).toEqual(["defaultWorkerFactory"]);
  expect(RecordedWorker.started).toEqual([
    {
      scriptURL: { policy: "defaultWorkerFactory", value: expect.stringContaining("json.worker") as string },
      options: { name: "json" },
    },
    {
      scriptURL: { policy: "defaultWorkerFactory", value: expect.stringContaining("editor.worker") as string },
      options: { name: "editorWorkerService" },
    },
  ]);
});

test("a worker starts from its plain script URL in a browser without Trusted Types", async () => {
  const environment = await installedEnvironment();

  await environment.getWorker?.("workerMain.js", "json");

  expect(RecordedWorker.started).toEqual([{ scriptURL: expect.stringContaining("json.worker") as string, options: { name: "json" } }]);
});
