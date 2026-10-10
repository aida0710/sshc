// Keep each demo's Ethernet frames within this page, including when another tab is open.
export function connectVirtualLAN(machines) {
  for (const machine of machines) {
    machine.add_listener("net0-send", (packet) => {
      for (const peer of machines) {
        if (peer !== machine) peer.bus.send("net0-receive", packet.slice());
      }
    });
  }
}
