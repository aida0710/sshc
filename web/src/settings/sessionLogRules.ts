// The session log rules as the engine reads them (internal/sessionlog CompileRules): lines of connection name patterns
// in the way of a .gitignore. This tells only whether the engine takes them, so a half-typed pattern is not saved.

// maxSessionLogRulesBytes is the engine's MaxRulesBytes.
const maxSessionLogRulesBytes = 8 << 10;

// sessionLogRulesValid reports whether every line is a pattern: a [ ] class closed, and something to match.
export function sessionLogRulesValid(text: string): boolean {
  if (new TextEncoder().encode(text).length > maxSessionLogRulesBytes || text.includes("\u0000")) return false;
  return text.split(/\r?\n/).every((raw) => {
    let line = raw.trim();
    if (line === "" || line.startsWith("#")) return true;
    if (line.startsWith("\\#") || line.startsWith("\\!")) line = line.slice(1);
    else if (line.startsWith("!")) line = line.slice(1).trim();
    return line !== "" && classesClosed(line);
  });
}

// classesClosed reports whether every [ starts a class that a ] closes with something in it, as Go's path.Match
// requires; \ takes the next character as it is.
function classesClosed(pattern: string): boolean {
  let inClass = false;
  let members = 0;
  for (let index = 0; index < pattern.length; index++) {
    const character = pattern[index];
    if (character === "\\") {
      index++;
      if (index >= pattern.length) return false;
      if (inClass) members++;
    } else if (character === "[" && !inClass) {
      inClass = true;
      members = 0;
      if (pattern[index + 1] === "^") index++;
    } else if (character === "]" && inClass) {
      if (members === 0) return false;
      inClass = false;
    } else if (inClass) {
      members++;
    }
  }
  return !inClass;
}
