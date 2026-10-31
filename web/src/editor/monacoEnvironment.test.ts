import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { Environment, ITrustedTypePolicyOptions } from "monaco-editor/editor/editor.api.js";

// A trusted value as the fake browser below makes it: the value a policy let
// through, with the name of that policy.
type TrustedValue = { policy: string; value: string };

// A stand-in for the browser's Trusted Types factory under the engine's
// policy, which carries no 'allow-duplicates': a name is created only once. It
// records the name of every policy it creates.
function fakeTrustedTypes() {
  const createdNames: string[] = [];
  return {
    createdNames,
    createPolicy(name: string, options: ITrustedTypePolicyOptions) {
      if (createdNames.includes(name)) throw new TypeError(`Policy with name "${name}" already exists.`);
      createdNames.push(name);
      return {
        name,
        createHTML: (value: string): TrustedValue => ({ policy: name, value: options.createHTML?.(value) ?? "" }),
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
  vi.unstubAllEnvs();
  globalThis.MonacoEnvironment = undefined;
});

test("a policy Monaco asks for is created by the browser with Monaco's rules, and asking for its name again is refused", async () => {
  const trustedTypes = fakeTrustedTypes();
  vi.stubGlobal("trustedTypes", trustedTypes);
  const environment = await installedEnvironment();

  const policy = environment.createTrustedTypesPolicy?.("editorViewLayer", { createHTML: (value) => `<span>${value}</span>` });

  expect(policy?.createHTML?.("line")).toEqual({ policy: "editorViewLayer", value: "<span>line</span>" });
  expect(() => environment.createTrustedTypesPolicy?.("editorViewLayer", { createHTML: () => "" })).toThrow("already exists");
});

test("Monaco gets no worker policy, so the browser creates that policy once however many Monaco modules ask for it", async () => {
  const trustedTypes = fakeTrustedTypes();
  vi.stubGlobal("trustedTypes", trustedTypes);
  const environment = await installedEnvironment();

  const fromEditor = environment.createTrustedTypesPolicy?.("defaultWorkerFactory", { createScriptURL: (value) => value });
  const fromJSONSupport = environment.createTrustedTypesPolicy?.("defaultWorkerFactory", { createScriptURL: (value) => value });

  expect(fromEditor).toBeUndefined();
  expect(fromJSONSupport).toBeUndefined();
  expect(trustedTypes.createdNames).toEqual(["defaultWorkerFactory"]);
});

test("a worker starts from its bundled script URL made trusted by the worker policy", async () => {
  const trustedTypes = fakeTrustedTypes();
  vi.stubGlobal("trustedTypes", trustedTypes);
  const environment = await installedEnvironment();

  await environment.getWorker?.("workerMain.js", "json");
  await environment.getWorker?.("workerMain.js", "editorWorkerService");

  expect(trustedTypes.createdNames).toEqual(["defaultWorkerFactory"]);
  expect(RecordedWorker.started).toEqual([
    {
      scriptURL: { policy: "defaultWorkerFactory", value: expect.stringContaining("json.worker") as string },
      options: expect.objectContaining({ name: "json" }) as object,
    },
    {
      scriptURL: { policy: "defaultWorkerFactory", value: expect.stringContaining("editor.worker") as string },
      options: expect.objectContaining({ name: "editorWorkerService" }) as object,
    },
  ]);
});

test("a worker starts from its plain script URL in a browser without Trusted Types", async () => {
  const environment = await installedEnvironment();

  await environment.getWorker?.("workerMain.js", "json");

  expect(RecordedWorker.started).toEqual([{ scriptURL: expect.stringContaining("json.worker") as string, options: expect.objectContaining({ name: "json" }) as object }]);
});

test("a worker starts as a classic script in a build and as an ES module from the dev server, as Vite makes it", async () => {
  vi.stubEnv("DEV", false);
  const built = await installedEnvironment();
  await built.getWorker?.("workerMain.js", "json");

  globalThis.MonacoEnvironment = undefined;
  vi.stubEnv("DEV", true);
  const served = await installedEnvironment();
  await served.getWorker?.("workerMain.js", "json");

  expect(RecordedWorker.started.map(({ options }) => options)).toEqual([
    { type: "classic", name: "json" },
    { type: "module", name: "json" },
  ]);
});
