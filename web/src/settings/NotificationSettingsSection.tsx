import { useEffect, useState } from "react";
import { usePresetInterfaceEdit, useSettingsDraft } from "./settingsDraftContext";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import {
  notificationSoundOf,
  notificationSoundPresets,
  playNotificationSound,
  withNotificationSound,
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
  const preset = useSettingsDraft().document?.presets.find((candidate) => candidate.id === presetId);
  const sounds = notificationSoundOf(preset ?? { id: presetId });
  const editInterface = usePresetInterfaceEdit(presetId);

  useEffect(() => {
    const refreshPermission = () => setPermission(browserNotificationPermission());
    window.addEventListener("focus", refreshPermission);
    return () => window.removeEventListener("focus", refreshPermission);
  }, []);

  function changeSounds(next: NotificationSoundPreferences) {
    editInterface("notificationSound", (current) => withNotificationSound(current, next));
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
        {/* 許可されていることは上の文が言うので、操作の下には失敗だけを出す。 */}
        {permission === "default" || permission === "granted" || request.error !== "" ? (
          <ActionArea status={request.error === "" ? undefined : <Notice tone="danger">{request.error}</Notice>}
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
          <FieldRow
            label={t("terminal.notificationSound")}
            hint={t(notificationsComeFromAndroid() ? "terminal.androidNotificationSoundHint" : "terminal.notificationSoundHint")}
          >
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
