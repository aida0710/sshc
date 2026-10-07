export class VMBridge {
  #serialLine = "";
  #listeners = new Set();
  #decoder = new TextDecoder();

  constructor(machine) {
    this.machine = machine;
    machine.add_listener("serial1-output-byte", (byte) => {
      if (byte !== 10) {
        this.#serialLine += this.#decoder.decode(Uint8Array.of(byte), { stream: true });
        return;
      }
      try {
        const reply = JSON.parse(this.#serialLine);
        for (const listener of this.#listeners) listener(reply);
      } catch {
        // UART initialization can emit non-protocol bytes before the bridge starts.
      }
      this.#serialLine = "";
    });
  }

  subscribe(listener) {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  send(request) {
    const line = new TextEncoder().encode(JSON.stringify(request) + "\n");
    for (const byte of line) {
      this.machine.bus.send("serial1-input", byte);
    }
  }
}
