// ReadScope ends the reads of an engine nobody is looking at any more. A read to a run target that does not answer
// otherwise goes on holding a browser connection, and the poll it belongs to, long after the screen moved on.
export type ReadScope = {
  signal: () => AbortSignal;
  // end aborts the reads running now; those started later run as usual. React's StrictMode ends a scope once on
  // mounting and goes on using it, so the scope has to survive that.
  end: () => void;
};

export function createReadScope(): ReadScope {
  let controller = new AbortController();
  return {
    signal: () => controller.signal,
    end: () => {
      controller.abort();
      controller = new AbortController();
    },
  };
}

// readOptions are the request options of a read that ends with `reads`, when there is such a scope.
export function readOptions(reads: ReadScope | undefined): { signal?: AbortSignal } {
  return reads === undefined ? {} : { signal: reads.signal() };
}

// wasAborted says whether a request failed because a ReadScope ended it, not because of anything to report.
export function wasAborted(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
