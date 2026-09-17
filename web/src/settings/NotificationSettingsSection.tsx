import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import {
  browserNotificationPermission,
  loadNotificationSoundPreferences,
  notificationSoundPresets,
  playNotificationSound,
  requestBrowserNotificationPermission,
  saveNotificationSoundPreferences,
  showBrowserNotification,
  type BrowserNotificationPermission,
  type NotificationSoundPreferences,
} from "../terminal/terminalNotifications";
import { Field, control } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";

// Browser notifications for finished commands: the permission the browser
// granted, and the sound this browser plays. Neither reaches the engine.
export function NotificationSettingsSection({ showHeading }: { showHeading: boolean }) {
  const t = useTranslate();
  const [permission, setPermission] = useState<BrowserNotificationPermission>(() => browserNotificationPermission());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [sounds, setSounds] = useState<NotificationSoundPreferences>(() => loadNotificationSoundPreferences());

  useEffect(() => {
    const refreshPermission = () => setPermission(browserNotificationPermission());
    window.addEventListener("focus", refreshPermission);
    return () => window.removeEventListener("focus", refreshPermission);
  }, []);

  function changeSounds(next: NotificationSoundPreferences) {
    setSounds(next);
    saveNotificationSoundPreferences(next);
  }

  async function enableOrTest() {
    setBusy(true);
    setError("");
    try {
      const granted = await requestBrowserNotificationPermission();
      setPermission(granted);
      if (granted === "granted") {
        const delivered = showBrowserNotification({
          title: "sshc",
          body: t("terminal.browserNotificationsReady"),
          tag: "sshc-notification-permission",
        });
        if (!delivered) setError(t("terminal.browserNotificationsDeliveryFailed"));
      }
    } catch {
      setPermission(browserNotificationPermission());
      setError(t("terminal.browserNotificationsRequestFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SettingsSection
      id="settings-notifications"
      label={t("terminal.browserNotificationsHeading")}
      icon="notification"
      showHeading={showHeading}
    >
      <div className="grid max-w-2xl gap-5">
        <p className="text-sm leading-6 text-ink-muted">
          {t(
            permission === "granted"
              ? "terminal.browserNotificationsGranted"
              : permission === "denied"
                ? "terminal.browserNotificationsDenied"
                : permission === "unsupported"
                  ? "terminal.browserNotificationsUnsupported"
                  : "terminal.browserNotificationsDefault",
          )}
        </p>
        {permission === "default" || permission === "granted" || error !== "" ? (
          <ActionArea status={error === ""
            ? (permission === "granted"
                ? <p role="status" className="text-sm text-live">{t("terminal.browserNotificationsEnabled")}</p>
                : undefined)
            : <Notice tone="danger">{error}</Notice>}
          >
            {permission === "default" || permission === "granted" ? (
              <Button
                kind={permission === "default" ? "primary" : "secondary"}
                disabled={busy}
                onClick={() => void enableOrTest()}
              >
                {t(permission === "granted"
                  ? "terminal.browserNotificationsTest"
                  : "terminal.browserNotificationsEnable")}
              </Button>
            ) : null}
          </ActionArea>
        ) : null}
        <div className="grid gap-4 border-t border-line pt-5 sm:grid-cols-2">
          <Field label={t("terminal.notificationSound")} hint={t("terminal.notificationSoundHint")}>
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
                  <option key={preset} value={preset}>{t(`terminal.notificationSound.${preset}`)}</option>
                ))}
              </select>
              <Button
                aria-label={t("terminal.notificationPreviewSound")}
                onClick={() => playNotificationSound(sounds)}
              >
                {t("terminal.notificationPreview")}
              </Button>
            </div>
          </Field>
          <Field
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
              className="h-9 w-full accent-accent"
            />
          </Field>
        </div>
      </div>
    </SettingsSection>
  );
}
