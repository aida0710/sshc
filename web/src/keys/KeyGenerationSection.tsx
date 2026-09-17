import { useState } from "react";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "./workflow";
import { keysApi, type KeyVariant, type KeysApi } from "./api";
import { useTranslate } from "../i18n/context";
import { CopyButton } from "../ui/CopyButton";
import { CheckboxField, control, hintText, sectionCard, sectionHeading } from "../ui/form";
import { PasswordInput } from "../ui/PasswordField";
import { Button, Card, Row } from "../ui/surface";
import { useGenerationForm } from "./forms";

// Creating a key: in the engine for software algorithms, or as a terminal
// command to run by hand for hardware-backed ones. A generated key is offered
// to the connection form and to the server installer right away.
export function KeyGenerationSection({
  api = keysApi, variants, groups, onGenerated, onFailure, onAssignGeneratedKey, onInstallGeneratedKey,
}: {
  api?: Pick<KeysApi, "generate" | "hardwareCommand">;
  variants: KeyVariant[];
  groups: string[];
  onGenerated: () => Promise<void>;
  onFailure: (message: string) => void;
  onAssignGeneratedKey?: ((key: GeneratedPrivateKeyHandoff) => void) | undefined;
  onInstallGeneratedKey?: ((key: GeneratedPublicKeyHandoff) => void) | undefined;
}) {
  const t = useTranslate();
  const {
    algorithm, setAlgorithm, fileName, setFileName, comment, setComment,
    passphrase, setPassphrase, unencrypted, setUnencrypted, createGroup, setCreateGroup,
  } = useGenerationForm();
  const [terminalCommand, setTerminalCommand] = useState<string[] | null>(null);
  const [generated, setGenerated] = useState<{
    private: GeneratedPrivateKeyHandoff;
    public: GeneratedPublicKeyHandoff;
  } | null>(null);
  const selected = variants.find((variant) => variant.algorithm === algorithm);
  const inProcess = selected === undefined || selected.inProcess;

  async function submitGeneration() {
    onFailure("");
    setTerminalCommand(null);
    setGenerated(null);
    try {
      if (selected !== undefined && !selected.inProcess) {
        const response = await api.hardwareCommand({
          algorithm,
          fileName,
          group: createGroup,
          comment,
        });
        setTerminalCommand(response.command);
        return;
      }
      const response = await api.generate({
        algorithm,
        bits: selected?.bits ?? 0,
        fileName,
        group: createGroup,
        comment,
        passphrase,
        unencrypted,
      });
      setGenerated({
        private: {
          privateKeyId: response.id,
          privateRelativePath: response.relativePath,
        },
        public: { publicRelativePath: response.publicRelativePath },
      });
      setPassphrase("");
      setFileName("");
      await onGenerated();
    } catch {
      setPassphrase("");
      onFailure(t("keys.createFailed"));
    }
  }

  return (
    <>
      <form
        aria-labelledby="create-key-heading"
        className="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void submitGeneration();
        }}
      >
        <h3 id="create-key-heading" className={sectionHeading}>
          {t("keys.createHeading")}
        </h3>
        <Card>
          <Row label={t("keys.algorithm")} stackOnNarrow>
            <select
              className={`${control} min-h-10 sm:min-h-0`}
              value={algorithm}
              onChange={(event) => setAlgorithm(event.target.value)}
            >
              {variants.map((variant) => (
                <option
                  key={`${variant.algorithm}-${variant.bits}`}
                  value={variant.algorithm}
                >
                  {variant.label}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.createGroup")} stackOnNarrow>
            <select
              className={`${control} min-h-10 sm:min-h-0`}
              value={createGroup}
              onChange={(event) => setCreateGroup(event.target.value)}
            >
              <option value="">{t("keys.groupNone")}</option>
              {groups.map((group) => (
                <option key={group} value={group}>
                  {group}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.fileName")} stackOnNarrow>
            <input
              className={`${control} min-h-10 sm:min-h-0`}
              value={fileName}
              onChange={(event) => setFileName(event.target.value)}
            />
          </Row>
          <Row label={t("keys.comment")} stackOnNarrow>
            <input
              className={`${control} min-h-10 sm:min-h-0`}
              value={comment}
              onChange={(event) => setComment(event.target.value)}
            />
          </Row>
          {inProcess && (
            <Row label={t("keys.passphrase")} stackOnNarrow interactiveChildren>
              <PasswordInput
                label={t("keys.passphrase")}
                className={`${control} min-h-10 sm:min-h-0`}
                value={passphrase}
                onChange={setPassphrase}
                disabled={unencrypted}
              />
            </Row>
          )}
        </Card>
        {inProcess && (
          <CheckboxField
            label={t("keys.createUnencrypted")}
            checked={unencrypted}
            onChange={(checked) => {
              setUnencrypted(checked);
              setPassphrase("");
            }}
          />
        )}
        <Button kind="primary" type="submit" className="self-start">
          {inProcess ? t("keys.createSubmit") : t("keys.showTerminalCommand")}
        </Button>
      </form>

      {generated === null ? null : (
        <section aria-live="polite" className={sectionCard}>
          <h3 className={sectionHeading}>{t("keys.generatedHeading")}</h3>
          <p className={hintText}>
            {t("keys.generatedNext", {
              path: generated.private.privateRelativePath,
            })}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              kind="primary"
              onClick={() => onAssignGeneratedKey?.(generated.private)}
            >
              {t("keys.assignGenerated")}
            </Button>
            <Button onClick={() => onInstallGeneratedKey?.(generated.public)}>
              {t("keys.installGenerated")}
            </Button>
          </div>
        </section>
      )}

      {terminalCommand !== null && (
        <div>
          <p className="text-sm text-ink-muted">{t("keys.hardwareNote")}</p>
          <pre
            aria-label={t("copy.terminalCommand")}
            className="overflow-x-auto rounded-md bg-canvas p-4 text-xs"
          >
            {terminalCommand.join(" ")}
          </pre>
          <div className="mt-2">
            <CopyButton
              value={terminalCommand.join(" ")}
              label="copy.terminalCommand"
            />
          </div>
        </div>
      )}
    </>
  );
}
