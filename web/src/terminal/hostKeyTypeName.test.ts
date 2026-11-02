import { expect, it } from "vitest";
import { hostKeyTypeName } from "./hostKeyTypeName";

it("names a host key type the way OpenSSH asks about it", () => {
  expect(hostKeyTypeName("ssh-ed25519")).toBe("ED25519");
  expect(hostKeyTypeName("ssh-rsa")).toBe("RSA");
  expect(hostKeyTypeName("ecdsa-sha2-nistp256")).toBe("ECDSA");
  expect(hostKeyTypeName("sk-ssh-ed25519@openssh.com")).toBe("ED25519-SK");
  expect(hostKeyTypeName("sk-ecdsa-sha2-nistp256@openssh.com")).toBe("ECDSA-SK");
});
