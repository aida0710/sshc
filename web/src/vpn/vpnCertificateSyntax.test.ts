import { describe, expect, it } from "vitest";
import profileCases from "../../../internal/vpn/testdata/profile-cases.json";
import { isPEMCertificateList } from "./vpnCertificateSyntax";

// certificate は、共有の表で engine が受け付ける CA の証明書である。
const certificate = profileCases.cases
  .flatMap((test) => ("ikev2" in test.profile ? [test.profile.ikev2?.caCertificate ?? ""] : []))
  .find((value) => value !== "") ?? "";

describe("isPEMCertificateList", () => {
  it("accepts one or more certificates separated by blank space", () => {
    expect(isPEMCertificateList(certificate)).toBe(true);
    expect(isPEMCertificateList(`\n${certificate}\n\n${certificate}  \n`)).toBe(true);
  });

  it("refuses anything that is not only certificates", () => {
    expect(isPEMCertificateList("")).toBe(false);
    expect(isPEMCertificateList(`CA:\n${certificate}`)).toBe(false);
    expect(isPEMCertificateList(`${certificate}trailing`)).toBe(false);
    expect(isPEMCertificateList(certificate.replaceAll("CERTIFICATE", "PRIVATE KEY"))).toBe(false);
    expect(isPEMCertificateList(certificate.replace("-----END CERTIFICATE-----", ""))).toBe(false);
  });

  it("refuses a body that is not a single DER SEQUENCE", () => {
    expect(isPEMCertificateList("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n")).toBe(false);
    // SEQUENCE のあとに余分なバイトがある（30 03 02 01 01 00）。
    expect(isPEMCertificateList("-----BEGIN CERTIFICATE-----\nMAMCAQEA\n-----END CERTIFICATE-----\n")).toBe(false);
    expect(isPEMCertificateList("-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n")).toBe(false);
  });

  // X.509 として読めるかは engine だけが確かめる。SEQUENCE の形が正しければ、ここは通す。
  it("leaves the X.509 contents to the engine", () => {
    expect(isPEMCertificateList("-----BEGIN CERTIFICATE-----\nMAMCAQE=\n-----END CERTIFICATE-----\n")).toBe(true);
  });
});
