// How many lines the browser's xterm keeps for each terminal. The engine
// checks the saved setting against the same range (MinBrowserScrollbackLines,
// MaxBrowserScrollbackLines and DefaultBrowserScrollbackLines in
// internal/terminal), and a blank setting falls back to the default here.
export const minBrowserScrollbackLines = 1000;
export const maxBrowserScrollbackLines = 100_000;
export const defaultBrowserScrollbackLines = 5000;
