import process from "node:process";

const mode = process.argv[2];
if (mode !== "prepare" && mode !== "verify-and-pull" && mode !== "run-configured") {
  throw new Error("usage: node sync-runtime-test.mjs <prepare|verify-and-pull|run-configured>");
}

const endpoint = process.env.SSHC_WEBVIEW_DEBUG_ENDPOINT ?? "http://127.0.0.1:9222";
const locale = process.env.SSHC_SYNC_TEST_LOCALE ?? "";
if (locale !== "" && locale !== "en" && locale !== "ja") {
  throw new Error("SSHC_SYNC_TEST_LOCALE must be en or ja");
}
const fields = {
  endpoint: process.env.SSHC_SYNC_TEST_ENDPOINT ?? "",
  bucket: process.env.SSHC_SYNC_TEST_BUCKET ?? "",
  accessKey: process.env.SSHC_SYNC_TEST_ACCESS_KEY_ID ?? "",
  secretKey: process.env.SSHC_SYNC_TEST_SECRET_ACCESS_KEY ?? "",
  syncKey: process.env.SSHC_SYNC_TEST_KEY ?? "",
};
if (mode !== "run-configured" && Object.values(fields).some((value) => value === "")) {
  throw new Error("the Android sync runtime test requires every SSHC_SYNC_TEST_* value");
}

async function waitForPage() {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      const response = await fetch(`${endpoint}/json`);
      const pages = await response.json();
      const page = pages.find((candidate) => candidate.type === "page" && candidate.webSocketDebuggerUrl);
      if (page) return page;
    } catch {
      // The app process starts before the WebView debugging socket.
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error("the sshc WebView did not publish a debuggable page");
}

function evaluate(socket, expression) {
  return new Promise((resolve, reject) => {
    const id = 1;
    const timer = setTimeout(() => reject(new Error("WebView evaluation timed out")), 120_000);
    socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.id !== id) return;
      clearTimeout(timer);
      if (message.error) reject(new Error(JSON.stringify(message.error)));
      else if (message.result.exceptionDetails) {
        reject(new Error(message.result.exceptionDetails.exception?.description ?? "WebView evaluation failed"));
      } else resolve(message.result.result);
    });
    socket.send(JSON.stringify({
      id,
      method: "Runtime.evaluate",
      params: { expression, awaitPromise: true, returnByValue: true },
    }));
  });
}

const expression = `
  (async () => {
    const mode = ${JSON.stringify(mode)};
    const expected = ${JSON.stringify(fields)};
    const expectedLocale = ${JSON.stringify(locale)};
    const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
    const waitFor = async (read, description) => {
      for (let attempt = 0; attempt < 240; attempt += 1) {
        const value = read();
        if (value) return value;
        const report = document.querySelector('pre')?.innerText ?? '';
        if (report.includes('Code:')) throw new Error(report);
        await sleep(250);
      }
      throw new Error('timed out waiting for ' + description + '; page=' + location.pathname + '; text=' + document.body.innerText.slice(0, 800));
    };
    const byLabel = (wanted, selector) => [...document.querySelectorAll('label')]
      .find((label) => wanted.some((text) => label.textContent.includes(text)))?.querySelector(selector);
    const input = (wanted) => byLabel(wanted, 'input');
    const setValue = (element, value) => {
      const prototype = element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(prototype, 'value').set.call(element, value);
      element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }));
    };
    const button = (labels) => [...document.querySelectorAll('button')]
      .find((candidate) => labels.includes(candidate.textContent.trim()));
    const problemCode = () => [...document.querySelectorAll('code')]
      .map((node) => node.textContent.trim())
      .find((text) => text.startsWith('Code:')) ?? '';
    const apiRead = (path) => {
      const csrf = sessionStorage.getItem('sshc.session.csrf');
      if (!csrf) throw new Error('the WebView session has no CSRF token');
      return fetch(path, { credentials: 'same-origin', headers: { 'X-SSHC-CSRF': csrf } });
    };
    const apiPost = (path, body = {}) => {
      const csrf = sessionStorage.getItem('sshc.session.csrf');
      if (!csrf) throw new Error('the WebView session has no CSRF token');
      return fetch(path, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-SSHC-CSRF': csrf },
        body: JSON.stringify(body),
      });
    };

    if (expectedLocale && localStorage.getItem('sshc.language') !== expectedLocale) {
      localStorage.setItem('sshc.language', expectedLocale);
      location.reload();
      await new Promise(() => {});
    }

    if (location.pathname !== '/sync') {
      history.pushState(null, '', '/sync');
      dispatchEvent(new PopStateEvent('popstate'));
    }

    if (mode === 'run-configured') {
      await waitFor(() => button(['Sync now', '今すぐ同期']), 'the manual sync button');
      const beforeResponse = await apiRead('/api/v1/sync/bucket');
      if (!beforeResponse.ok) throw new Error('bucket status before pull failed with HTTP ' + beforeResponse.status);
      const before = await beforeResponse.json();
      const run = await apiPost('/api/v1/sync/now');
      const runBody = await run.json();
      if (!run.ok) throw new Error('manual sync failed with HTTP ' + run.status + ': ' + (runBody.code ?? 'unknown'));
      const response = await apiRead('/api/v1/sync');
      const status = await response.json();
      const afterResponse = await apiRead('/api/v1/sync/bucket');
      if (!afterResponse.ok) throw new Error('bucket status after pull failed with HTTP ' + afterResponse.status);
      const after = await afterResponse.json();
      const remoteUnchanged = JSON.stringify(before.live ?? null) === JSON.stringify(after.live ?? null);
      if (!remoteUnchanged) throw new Error('receive-only changed the live remote object');
      return {
        ok: true,
        phase: mode,
        configured: status?.configured === true,
        direction: status?.direction,
        synced: status?.synced === true,
        autoPhase: status?.auto?.phase,
        detail: status?.auto?.detail ?? '',
        fileCount: status?.fileCount ?? 0,
        remoteUnchanged,
      };
    }

    const endpointInput = await waitFor(() => input(['Endpoint', 'エンドポイント']), 'the endpoint field');
    const bucketInput = input(['Bucket name', 'バケット名']);
    const accessInput = input(['Access key ID', 'アクセスキー ID']);
    const secretInput = input(['Secret access key', 'シークレットアクセスキー']);
    const direction = byLabel(['Direction', '同期の方向'], 'select');
    if (!bucketInput || !accessInput || !secretInput || !direction) throw new Error('the sync setup form is incomplete');

    if (mode === 'prepare') {
      setValue(endpointInput, expected.endpoint);
      setValue(bucketInput, expected.bucket);
      setValue(accessInput, expected.accessKey);
      setValue(secretInput, expected.secretKey);
      setValue(direction, 'pull');
      await sleep(250);
      return { ok: true, phase: mode, path: location.pathname, retained: true };
    }

    const actual = [endpointInput.value, bucketInput.value, accessInput.value, secretInput.value, direction.value];
    const wanted = [expected.endpoint, expected.bucket, expected.accessKey, expected.secretKey, 'pull'];
    if (actual.some((value, index) => value !== wanted[index])) {
      const names = ['endpoint', 'bucket', 'access key', 'secret key', 'direction'];
      const changed = names.filter((_, index) => actual[index] !== wanted[index]);
      throw new Error('the sync form was recreated after the app switch; changed=' + changed.join(','));
    }

    const check = button(['Check connection', '接続を確認']);
    if (!check || check.disabled) throw new Error('the connection check is unavailable');
    check.click();
    const keyInput = await waitFor(
      () => document.querySelector('input[aria-label="Key"], input[aria-label="キー"]'),
      'the synchronization key field',
    );
    setValue(keyInput, expected.syncKey);
    const save = await waitFor(() => button(['Verify and save', '確認して保存']), 'the setup save button');
    if (save.disabled) throw new Error('the setup save button remained disabled');
    save.click();

    const syncNow = await waitFor(() => button(['Sync now', '今すぐ同期']), 'the manual sync button');
    const beforeResponse = await apiRead('/api/v1/sync/bucket');
    if (!beforeResponse.ok) throw new Error('bucket status before pull failed with HTTP ' + beforeResponse.status);
    const before = await beforeResponse.json();
    syncNow.click();
    let status;
    for (let attempt = 0; attempt < 240; attempt += 1) {
      const response = await apiRead('/api/v1/sync');
      status = await response.json();
      if (status.auto?.phase !== 'running' && status.synced === true) break;
      if (status.auto?.phase === 'failed' || status.auto?.phase === 'blocked') {
        throw new Error('manual receive stopped: ' + (status.auto.detail ?? 'unknown'));
      }
      const code = problemCode();
      if (code) throw new Error(code);
      await sleep(250);
    }
    if (status?.synced !== true || status?.direction !== 'pull') {
      throw new Error('manual receive did not finish in receive-only mode');
    }
    const afterResponse = await apiRead('/api/v1/sync/bucket');
    if (!afterResponse.ok) throw new Error('bucket status after pull failed with HTTP ' + afterResponse.status);
    const after = await afterResponse.json();
    const remoteUnchanged = JSON.stringify(before.live ?? null) === JSON.stringify(after.live ?? null);
    if (!remoteUnchanged) throw new Error('receive-only changed the live remote object');
    return {
      ok: true,
      phase: mode,
      path: location.pathname,
      retained: true,
      configured: status.configured === true,
      direction: status.direction,
      synced: status.synced,
      fileCount: status.fileCount,
      remoteUnchanged,
    };
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
if (result.subtype === "error") throw new Error(result.description ?? "WebView returned an error");
if (!result.value?.ok) throw new Error(`unexpected WebView result: ${JSON.stringify(result.value)}`);
process.stdout.write(`${JSON.stringify(result.value)}\n`);
