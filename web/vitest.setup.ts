import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// jsdom intentionally leaves canvas rendering unimplemented and emits a
// virtual-console error whenever xterm probes it. Returning null models an
// unsupported renderer, which is the browser fallback contract under test.
Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
  configurable: true,
  value: () => null,
});

afterEach(() => cleanup());
