// loadingPlaceholder marks what stands in for contents still being read: a page's skeleton, or its "reading"
// state. The Android app keeps its splash while one is in sight on the first screen (android/firstScreen.ts).
export const loadingPlaceholder = { "data-loading-placeholder": "" } as const;

export const loadingPlaceholderSelector = "[data-loading-placeholder]";
