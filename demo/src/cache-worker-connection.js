export async function connectCacheWorker(workerURL, { requireController = true } = {}) {
  const registration = await navigator.serviceWorker.register(workerURL, { type: "module", updateViaCache: "none" });
  if (!registration.active || registration.active.state !== "activated") {
    const worker = registration.installing ?? registration.waiting ?? registration.active;
    if (!worker) throw new Error("Demo cache worker is unavailable");
    await new Promise((resolve, reject) => {
      const checkState = () => {
        if (worker.state === "activated") resolve();
        else if (worker.state === "redundant") reject(new Error("Demo cache worker failed"));
      };
      worker.addEventListener("statechange", checkState);
      checkState();
    });
  }
  // A snapshot may have a more specific UI worker scope. The next navigation uses the root worker.
  if (!requireController || navigator.serviceWorker.controller?.scriptURL === workerURL.href) return;
  await new Promise((resolve) => {
    const checkController = () => {
      if (navigator.serviceWorker.controller?.scriptURL !== workerURL.href) return;
      navigator.serviceWorker.removeEventListener("controllerchange", checkController);
      resolve();
    };
    navigator.serviceWorker.addEventListener("controllerchange", checkController);
    checkController();
  });
}
