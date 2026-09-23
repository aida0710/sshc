import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { vpnApi, type VPNApi, type VPNOverview, type VPNProfile, type VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Button, Card, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { vpnRefusals } from "./vpnRefusals";

// 接続ごとのVPN経路の画面。トンネルはengineが持つコンテナの中にあり、ここでは
// 経路の定義と、いまの状態と、どの接続がそれを通るかを扱う。秘密は保存のときに
// 送るだけで、engineは決して返さない。

type VPNPanelProps = {
  api?: VPNApi;
  // aliases は、紐付けの相手に選べる接続である。
  aliases?: string[];
};

type DraftSecrets = {
  wireguardPrivateKey: string;
  l2tpPassword: string;
  ipsecPsk: string;
};

const emptySecrets: DraftSecrets = { wireguardPrivateKey: "", l2tpPassword: "", ipsecPsk: "" };

export function VPNPanel({ api = vpnApi, aliases = [] }: VPNPanelProps) {
  const t = useTranslate();
  const operation = useAsyncOperation();
  const [overview, setOverview] = useState<VPNOverview | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState("");

  const describe = useCallback(
    (error: unknown) => {
      const code = failureCode(error);
      const key = code === undefined ? undefined : vpnRefusals[code];
      return key === undefined ? t("vpn.failed") : t(key);
    },
    [t],
  );

  const run = operation.run;
  useEffect(() => {
    // 開いたときに一度だけ読む。以降は、操作の応答が最新の一覧を運ぶ。
    void run(() => api.vpnOverview(), { apply: setOverview, describe });
  }, [api, run, describe]);

  const act = useCallback(
    async (work: () => Promise<VPNOverview>) => {
      await operation.run(work, { apply: setOverview, describe });
    },
    [describe, operation],
  );

  if (overview === null) {
    return (
      <PanelState
        tone={operation.error === "" ? "loading" : "failed"}
        title={operation.error === "" ? t("vpn.loading") : operation.error}
      />
    );
  }

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("vpn.heading")} description={t("vpn.description")} />

      {overview.available ? null : (
        <Notice>{t("vpn.unavailable", { detail: overview.detail ?? "" })}</Notice>
      )}
      {operation.error === "" ? null : <Notice tone="danger">{operation.error}</Notice>}

      {overview.profiles.length === 0 ? (
        <p className={hintText}>{t("vpn.empty")}</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {overview.profiles.map((session) => (
            <li key={session.profile.name}>
              <Card as="article" padded aria-label={session.profile.name}>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="font-medium text-ink">{session.profile.name}</p>
                    <p className={hintText}>
                      {session.profile.backend} · {session.profile.target} ·{" "}
                      {session.relaySocket !== ""
                        ? t("vpn.stateUp")
                        : session.running
                          ? t("vpn.stateStarting")
                          : t("vpn.stateStopped")}
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={operation.busy || !overview.available}
                      onClick={() => void act(() => api.startVPNSession(session.profile.name))}
                    >
                      {t("vpn.connect")}
                    </Button>
                    <Button
                      disabled={operation.busy || !session.running}
                      onClick={() => void act(() => api.stopVPNSession(session.profile.name))}
                    >
                      {t("vpn.disconnect")}
                    </Button>
                    <Button
                      kind="danger"
                      disabled={operation.busy}
                      onClick={() => setPendingRemoval(session.profile.name)}
                    >
                      {t("vpn.remove")}
                    </Button>
                  </div>
                </div>

                <BindingRow
                  profile={session.profile.name}
                  connections={session.connections}
                  aliases={aliases}
                  busy={operation.busy}
                  onBind={(alias) => void act(() => api.setConnectionVPN(alias, session.profile.name))}
                  onUnbind={(alias) => void act(() => api.setConnectionVPN(alias, ""))}
                />
              </Card>
            </li>
          ))}
        </ul>
      )}

      <ProfileForm
        busy={operation.busy}
        onSave={(profile, secrets) => void act(() => api.saveVPNProfile(profile, secrets))}
      />

      {pendingRemoval === "" ? null : (
        <ConfirmDialog
          id="vpn-remove"
          heading={t("vpn.removeTitle", { name: pendingRemoval })}
          body={t("vpn.removeBody")}
          confirmLabel={t("vpn.remove")}
          cancelLabel={t("vpn.cancel")}
          onCancel={() => setPendingRemoval("")}
          onConfirm={() => {
            const name = pendingRemoval;
            setPendingRemoval("");
            void act(() => api.removeVPNProfile(name));
          }}
        />
      )}
    </section>
  );
}

// BindingRow は、この経路を通る接続を見せ、増やしたり外したりする。
function BindingRow({
  profile,
  connections,
  aliases,
  busy,
  onBind,
  onUnbind,
}: {
  profile: string;
  connections: string[];
  aliases: string[];
  busy: boolean;
  onBind: (alias: string) => void;
  onUnbind: (alias: string) => void;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const available = aliases.filter((name) => !connections.includes(name));
  return (
    <div className="flex flex-col gap-2 border-t border-line pt-3">
      <p className={sectionHeading}>{t("vpn.connections")}</p>
      {connections.length === 0 ? (
        <p className={hintText}>{t("vpn.noConnections")}</p>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {connections.map((name) => (
            <li key={name} className="flex items-center gap-1 rounded bg-surface-subtle px-2 py-1 text-sm">
              <span>{name}</span>
              <button
                type="button"
                aria-label={t("vpn.unbindAction", { alias: name, name: profile })}
                className="text-ink-muted hover:text-ink"
                disabled={busy}
                onClick={() => onUnbind(name)}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-end gap-2">
        <Field label={t("vpn.bindLabel")}>
          <select
            className={control.replace("w-full", "w-56")}
            value={alias}
            disabled={busy || available.length === 0}
            onChange={(event) => setAlias(event.target.value)}
          >
            <option value="">{t("vpn.bindChoose")}</option>
            {available.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </Field>
        <Button
          disabled={busy || alias === ""}
          onClick={() => {
            onBind(alias);
            setAlias("");
          }}
        >
          {t("vpn.bindAction")}
        </Button>
      </div>
    </div>
  );
}

// ProfileForm は、経路をひとつ作る。接続先はひとつだけ持つ。
function ProfileForm({
  busy,
  onSave,
}: {
  busy: boolean;
  onSave: (profile: VPNProfile, secrets: VPNSecrets) => void;
}) {
  const t = useTranslate();
  const [name, setName] = useState("");
  const [backend, setBackend] = useState<"wireguard" | "l2tp_ipsec">("wireguard");
  const [target, setTarget] = useState("");
  const [server, setServer] = useState("");
  const [peerPublicKey, setPeerPublicKey] = useState("");
  const [address, setAddress] = useState("10.0.0.2/32");
  const [username, setUsername] = useState("");
  const [ike, setIke] = useState("");
  const [esp, setEsp] = useState("");
  const [secrets, setSecrets] = useState<DraftSecrets>(emptySecrets);

  const complete =
    name !== "" &&
    target !== "" &&
    server !== "" &&
    (backend === "wireguard"
      ? peerPublicKey !== "" && address !== "" && secrets.wireguardPrivateKey !== ""
      : username !== "" && secrets.l2tpPassword !== "" && secrets.ipsecPsk !== "");

  function save() {
    const profile: VPNProfile =
      backend === "wireguard"
        ? { name, backend, target, wireguard: { server, peerPublicKey, address } }
        : {
            name,
            backend,
            target,
            l2tp: {
              server,
              username,
              ...(ike === "" ? {} : { ike }),
              ...(esp === "" ? {} : { esp }),
            },
          };
    const carried: VPNSecrets =
      backend === "wireguard"
        ? { wireguardPrivateKey: secrets.wireguardPrivateKey }
        : { l2tpPassword: secrets.l2tpPassword, ipsecPsk: secrets.ipsecPsk };
    onSave(profile, carried);
    setSecrets(emptySecrets);
  }

  return (
    <Card as="section" padded aria-label={t("vpn.addHeading")}>
      <p className={sectionHeading}>{t("vpn.addHeading")}</p>
      <p className={hintText}>{t("vpn.addHint")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("vpn.name")}>
          <input className={control} value={name} onChange={(event) => setName(event.target.value)} />
        </Field>
        <Field label={t("vpn.backend")}>
          <select
            className={control}
            value={backend}
            onChange={(event) =>
              setBackend(event.target.value === "l2tp_ipsec" ? "l2tp_ipsec" : "wireguard")
            }
          >
            <option value="wireguard">WireGuard</option>
            <option value="l2tp_ipsec">L2TP/IPsec</option>
          </select>
        </Field>
        <Field label={t("vpn.target")} hint={t("vpn.targetHint")}>
          <input className={control} value={target} onChange={(event) => setTarget(event.target.value)} />
        </Field>
        <Field label={t("vpn.server")}>
          <input className={control} value={server} onChange={(event) => setServer(event.target.value)} />
        </Field>
        {backend === "wireguard" ? (
          <>
            <Field label={t("vpn.peerPublicKey")}>
              <input
                className={control}
                value={peerPublicKey}
                onChange={(event) => setPeerPublicKey(event.target.value)}
              />
            </Field>
            <Field label={t("vpn.address")}>
              <input className={control} value={address} onChange={(event) => setAddress(event.target.value)} />
            </Field>
            <Field label={t("vpn.privateKey")} hint={t("vpn.secretHint")}>
              <input
                type="password"
                className={control}
                value={secrets.wireguardPrivateKey}
                onChange={(event) => setSecrets({ ...secrets, wireguardPrivateKey: event.target.value })}
              />
            </Field>
          </>
        ) : (
          <>
            <Field label={t("vpn.username")}>
              <input className={control} value={username} onChange={(event) => setUsername(event.target.value)} />
            </Field>
            <Field label={t("vpn.password")} hint={t("vpn.secretHint")}>
              <input
                type="password"
                className={control}
                value={secrets.l2tpPassword}
                onChange={(event) => setSecrets({ ...secrets, l2tpPassword: event.target.value })}
              />
            </Field>
            <Field label={t("vpn.psk")} hint={t("vpn.secretHint")}>
              <input
                type="password"
                className={control}
                value={secrets.ipsecPsk}
                onChange={(event) => setSecrets({ ...secrets, ipsecPsk: event.target.value })}
              />
            </Field>
            <Field label={t("vpn.ike")} hint={t("vpn.proposalsHint")}>
              <input className={control} value={ike} onChange={(event) => setIke(event.target.value)} />
            </Field>
            <Field label={t("vpn.esp")} hint={t("vpn.proposalsHint")}>
              <input className={control} value={esp} onChange={(event) => setEsp(event.target.value)} />
            </Field>
          </>
        )}
      </div>
      <div>
        <Button kind="primary" disabled={busy || !complete} onClick={save}>
          {t("vpn.save")}
        </Button>
      </div>
    </Card>
  );
}
