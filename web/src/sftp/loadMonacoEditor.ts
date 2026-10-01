import { installMonacoEnvironment } from "./monacoEnvironment";

// loadMonacoEditor is the one way to load MonacoEditor.tsx. Monaco reads
// MonacoEnvironment, and creates its Trusted Types policies, while its modules
// are evaluated, so the environment is installed before the first import.
export function loadMonacoEditor(): Promise<typeof import("./MonacoEditor")> {
  installMonacoEnvironment();
  return import("./MonacoEditor");
}
