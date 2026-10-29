import { useEffect, useState } from "react";
import { aboutApi, type AboutApi } from "../api/about";

// useEngineOS is the operating system this browser's sshc engine runs on (About's os: "linux", "windows", …), or
// null until it is known or when it could not be read. enabled = false asks nothing.
export function useEngineOS(enabled = true, api: AboutApi = aboutApi): string | null {
  const [os, setOS] = useState<string | null>(null);
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    void api.about().then((about) => {
      if (!cancelled) setOS(about.os);
    }).catch(() => { if (!cancelled) setOS(null); });
    return () => { cancelled = true; };
  }, [enabled, api]);
  return os;
}
