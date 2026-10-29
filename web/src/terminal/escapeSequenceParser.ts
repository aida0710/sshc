import type { IParser } from "@xterm/xterm";

// The parts of xterm's parser that the escape-sequence handlers in this folder register with. Each handler
// asks only for the kinds it registers, so a test can hand it a fake with just those.
export type OSCParser = Pick<IParser, "registerOscHandler">;
export type CSIParser = Pick<IParser, "registerCsiHandler">;
