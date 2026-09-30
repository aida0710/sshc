import { useMemo, useRef } from "react";

// The check a request makes when its answer arrives: false once the answer is
// stale and must be dropped instead of shown.
export type IsCurrentRequest = () => boolean;

export type RequestGeneration = {
  // begin retires every earlier request and returns the check for a new one,
  // so the latest request wins.
  begin: () => IsCurrentRequest;
  // observe returns a check that stays true until the next begin or retire,
  // for a read whose answer must yield to anything started after it.
  observe: () => IsCurrentRequest;
  // retire makes every earlier request stale without starting a new one, for
  // a close, a reset or a write whose answer supersedes reads in flight.
  retire: () => void;
};

// useRequestGeneration numbers the requests of one kind so that an answer
// which arrives after the user has moved on is dropped. The returned object is
// stable across renders, so it can sit in effect and callback dependencies.
export function useRequestGeneration(): RequestGeneration {
  const generation = useRef(0);
  return useMemo(() => {
    const observe = (): IsCurrentRequest => {
      const started = generation.current;
      return () => started === generation.current;
    };
    const retire = () => {
      generation.current += 1;
    };
    const begin = (): IsCurrentRequest => {
      retire();
      return observe();
    };
    return { begin, observe, retire };
  }, []);
}
