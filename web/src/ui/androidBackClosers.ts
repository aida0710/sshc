// What Android's Back closes before it leaves the page (useShellLayers decides the order): a layer
// (useDismissibleLayer), an open part of a page (useAndroidBackStop), and selected text. Those modules tell here when
// one opens or closes, so that the app can tell Android whether Back is the page's (android/backOwnership.ts).
type Listener = () => void;

const listeners = new Set<Listener>();

export function notifyAndroidBackClosersChanged() {
  for (const listener of listeners) listener();
}

// subscribeToAndroidBackClosers calls listener when a layer or an open part opens or closes, or the selected text
// changes, and returns how to stop.
export function subscribeToAndroidBackClosers(listener: Listener): () => void {
  listeners.add(listener);
  document.addEventListener("selectionchange", listener);
  return () => {
    listeners.delete(listener);
    document.removeEventListener("selectionchange", listener);
  };
}
