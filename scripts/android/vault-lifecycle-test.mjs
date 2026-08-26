import process from "node:process";

const mode = process.argv[2];
if (mode !== "create" && mode !== "unlock") {
  throw new Error("usage: node vault-lifecycle-test.mjs <create|unlock>");
}

const endpoint = process.env.SSHC_WEBVIEW_DEBUG_ENDPOINT ?? "http://127.0.0.1:9222";
const passphrase = "android runtime fixture password";

async function waitForPage() {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      const response = await fetch(`${endpoint}/json`);
      const pages = await response.json();
      const page = pages.find((candidate) => candidate.type === "page" && candidate.webSocketDebuggerUrl);
      if (page) return page;
    } catch {
      // WebView starts after the process and its debugging socket.
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error("the sshc WebView did not publish a debuggable page");
}

function evaluate(socket, expression) {
  return new Promise((resolve, reject) => {
    const id = 1;
    const timer = setTimeout(() => reject(new Error("WebView evaluation timed out")), 45_000);
    socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.id !== id) return;
      clearTimeout(timer);
      if (message.error) reject(new Error(JSON.stringify(message.error)));
      else if (message.result.exceptionDetails) {
        reject(new Error(message.result.exceptionDetails.exception?.description ?? "WebView evaluation failed"));
      }
      else resolve(message.result.result);
    });
    socket.send(JSON.stringify({
      id,
      method: "Runtime.evaluate",
      params: { expression, awaitPromise: true, returnByValue: true },
    }));
  });
}

const wantedInputs = mode === "create" ? 2 : 1;
const buttonLabels = mode === "create"
  ? ["Create the vault", "vault を作成"]
  : ["Open", "開く"];
const expression = `
  (async () => {
    const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
    let inputs = [];
    let button;
    for (let attempt = 0; attempt < 160; attempt += 1) {
      inputs = [...document.querySelectorAll('input[type="password"]')];
      button = [...document.querySelectorAll('button')].find((candidate) =>
        ${JSON.stringify(buttonLabels)}.includes(candidate.textContent.trim()));
      if (inputs.length === ${wantedInputs} && button) break;
      await sleep(250);
    }
    if (inputs.length !== ${wantedInputs} || !button) {
      throw new Error('unexpected vault form: inputs=' + inputs.length + '; text=' + document.body.innerText.slice(0, 600));
    }
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    for (const input of inputs) {
      setValue.call(input, ${JSON.stringify(passphrase)});
      input.dispatchEvent(new Event('input', { bubbles: true }));
    }
    await sleep(100);
    button = [...document.querySelectorAll('button')].find((candidate) =>
      ${JSON.stringify(buttonLabels)}.includes(candidate.textContent.trim()));
    if (!button || button.disabled) throw new Error('vault submit remained disabled');
    button.click();
    for (let attempt = 0; attempt < 160; attempt += 1) {
      const passwords = document.querySelectorAll('input[type="password"]');
      if (passwords.length === 0) return { ok: true, title: document.title, path: location.pathname };
      const report = document.querySelector('pre')?.innerText ?? '';
      if (report.includes('Code:')) throw new Error(report);
      await sleep(250);
    }
    throw new Error('vault form did not close');
  })()
`;

let result;
for (let attempt = 0; attempt < 20; attempt += 1) {
  const page = await waitForPage();
  const socket = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    socket.addEventListener("open", resolve, { once: true });
    socket.addEventListener("error", reject, { once: true });
  });
  try {
    result = await evaluate(socket, expression);
    socket.close();
    break;
  } catch (error) {
    socket.close();
    if (!String(error).includes("Execution context was destroyed")) throw error;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
}
if (result === undefined) throw new Error("the WebView kept navigating while the test attached");

if (result.subtype === "error") {
  throw new Error(result.description ?? "WebView returned an error");
}
if (!result.value?.ok) {
  throw new Error(`unexpected WebView result: ${JSON.stringify(result.value)}`);
}
process.stdout.write(`${JSON.stringify({ mode, ...result.value })}\n`);
