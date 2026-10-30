import { useEffect, useMemo, useState } from "react";
import { useRestingSave } from "./useRestingSave";
import { useSettings } from "./settingsStore";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import {
  notificationSoundOf,
  notificationSoundPresets,
  playNotificationSound,
  saveNotificationSoundPreferences,
  type NotificationSoundPreferences,
  type NotificationSoundPreset,
} from "../terminal/notificationSound";
import {
  browserNotificationPermission,
  notificationsComeFromAndroid,
  requestBrowserNotificationPermission,
  showBrowserNotification,
  type BrowserNotificationPermission,
} from "../ui/browserNotifications";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Button, Notice } from "../ui/surface";
import { ActionArea } from "../ui/ActionArea";
import { SettingsSection } from "./SettingsSection";
import { useAsyncOperation } from "../ui/useAsyncOperation";

const soundLabelKeys: Record<NotificationSoundPreset, MessageKey> = {
  none: "terminal.notificationSound.none",
  gentle: "terminal.notificationSound.gentle",
  bell: "terminal.notificationSound.bell",
  pulse: "terminal.notificationSound.pulse",
};

function permissionMessage(permission: BrowserNotificationPermission, android: boolean): MessageKey {
  if (permission === "unsupported") return "terminal.browserNotificationsUnsupported";
  if (android) {
    return permission === "granted"
      ? "terminal.androidNotificationsGranted"
      : permission === "denied" ? "terminal.androidNotificationsDenied" : "terminal.androidNotificationsDefault";
  }
  return permission === "granted"
    ? "terminal.browserNotificationsGranted"
    : permission === "denied" ? "terminal.browserNotificationsDenied" : "terminal.browserNotificationsDefault";
}

// Browser notifications for finished commands: the permission this browser granted, and the sound of the
// preset being edited.
export function NotificationSettingsSection({ presetId, showHeading }: { presetId: string; showHeading: boolean }) {
  const t = useTranslate();
  const [permission, setPermission] = useState<BrowserNotificationPermission>(() => browserNotificationPermission());
  const request = useAsyncOperation();
  const settingsDocument = useSettings().document;
  const preset = settingsDocument?.presets.find((candidate) => candidate.id === presetId);
  const storedSounds = useMemo(() => notificationSoundOf(preset ?? { id: presetId }), [preset, presetId]);
  // sounds is what the controls show while a change has not been saved yet.
  const [sounds, setSounds] = useState<NotificationSoundPreferences>(storedSounds);
  useEffect(() => setSounds(storedSounds), [storedSounds]);

  useEffect(() => {
    const refreshPermission = () => setPermission(browserNotificationPermission());
    window.addEventListener("focus", refreshPermission);
    return () => window.removeEventListener("focus", refreshPermission);
  }, []);

  const saveSounds = useRestingSave((next: NotificationSoundPreferences) => {
    void saveNotificationSoundPreferences(next, presetId).catch(() => undefined);
  });

  function changeSounds(next: NotificationSoundPreferences) {
    setSounds(next);
    saveSounds(next);
  }

  async function enableOrTest() {
    await request.run(async () => {
      const granted = await requestBrowserNotificationPermission();
      setPermission(granted);
      if (granted === "granted") {
        const delivered = showBrowserNotification({
          title: "sshc",
          body: t("terminal.browserNotificationsReady"),
          tag: "sshc-notification-permission",
        });
        if (!delivered) throw new Error(t("terminal.browserNotificationsDeliveryFailed"));
      }
    }, {
      describe: (error) => {
        // A permission request that threw leaves the browser's own answer as
        // the truth; a delivery failure keeps the granted permission.
        if (error instanceof Error && error.message === t("terminal.browserNotificationsDeliveryFailed")) return error.message;
        setPermission(browserNotificationPermission());
        return t("terminal.browserNotificationsRequestFailed");
      },
    });
  }

  return (
    <SettingsSection
      id="settings-notifications"
      label={t("terminal.browserNotificationsHeading")}
      icon="notification"
      showHeading={showHeading}
    >
      <div className="grid gap-5">
        {/* Blocked notifications need the person to act in the browser's settings, so they get a notice. */}
        {permission === "denied" ? (
          <Notice>{t(permissionMessage(permission, notificationsComeFromAndroid()))}</Notice>
        ) : (
          <p className="text-sm leading-6 text-ink-muted">
            {t(permissionMessage(permission, notificationsComeFromAndroid()))}
          </p>
        )}
        {permission === "default" || permission === "granted" || request.error !== "" ? (
          <ActionArea status={request.error === ""
            ? (permission === "granted"
                ? <Notice tone="success">{t("terminal.browserNotificationsEnabled")}</Notice>
                : undefined)
            : <Notice tone="danger">{request.error}</Notice>}
          >
            {permission === "default" || permission === "granted" ? (
              <Button
                kind={permission === "default" ? "primary" : "secondary"}
                disabled={request.busy}
                onClick={() => void enableOrTest()}
              >
                {t(permission === "granted"
                  ? "terminal.browserNotificationsTest"
                  : "terminal.browserNotificationsEnable")}
              </Button>
            ) : null}
          </ActionArea>
        ) : null}
        <div className="border-t border-line pt-5">
          <FieldRow label={t("terminal.notificationSound")} hint={t("terminal.notificationSoundHint")}>
            <div className="flex gap-2">
              <select
                className={control}
                value={sounds.sound}
                onChange={(event) => changeSounds({
                  ...sounds,
                  sound: event.target.value as NotificationSoundPreferences["sound"],
                })}
              >
                {notificationSoundPresets.map((preset) => (
                  <option key={preset} value={preset}>{t(soundLabelKeys[preset])}</option>
                ))}
              </select>
              <Button
                aria-label={t("terminal.notificationPreviewSound")}
                onClick={() => playNotificationSound(sounds)}
              >
                {t("terminal.notificationPreview")}
              </Button>
            </div>
          </FieldRow>
          <FieldRow
            label={t("terminal.notificationVolume")}
            hint={t("terminal.notificationVolumeHint", { volume: String(sounds.volume) })}
          >
            <input
              aria-label={t("terminal.notificationVolume")}
              type="range"
              min={0}
              max={100}
              step={5}
              value={sounds.volume}
              onChange={(event) => changeSounds({ ...sounds, volume: Number(event.target.value) })}
              className="h-9 w-full max-w-md accent-accent"
            />
          </FieldRow>
        </div>
      </div>
    </SettingsSection>
  );
}
