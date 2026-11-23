import { createContext, useContext } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";

// SessionAddressesContext holds the address (user@host:port) of each saved connection of the engine the sessions
// run on, by alias.
export const SessionAddressesContext = createContext<ReadonlyMap<string, string>>(new Map());

// SessionEngineContext is the remote session's engine the sessions below run on, by its alias; null is this machine.
// A shell is named after the machine it runs on.
export const SessionEngineContext = createContext<string | null>(null);

// useSessionDestination is the line under a session's name: where it connects. A session named after its
// alias would say the same thing twice, so that one says the address instead.
export function useSessionDestination(session: TerminalSession): string {
  const t = useTranslate();
  const addresses = useContext(SessionAddressesContext);
  const engine = useContext(SessionEngineContext);
  const destination = terminalSubtitle(session, t, engine);
  if (session.kind !== "ssh" || terminalDisplayTitle(session) !== destination) return destination;
  return addresses.get(destination) ?? destination;
}
