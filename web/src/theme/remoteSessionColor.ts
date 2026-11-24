import type { SettingsPreset } from "../settings/settingsStore";
import { isAccentColor, parseAccentColor } from "./accentColor";

// remoteSessionColorOf は、プリセットが決めたリモートセッションの枠の色。決めていなければ null で、さし色
// （テーマの --ui-remote）を使う。
export function remoteSessionColorOf(preset: SettingsPreset): string | null {
  const stored = preset.appearance?.remoteSessionColor;
  return isAccentColor(stored) ? stored : null;
}

// withRemoteSessionColor は、color を枠の色として持つプリセット。null はさし色に戻すことで、設定が無かったころに
// 保存したプリセットと同じく、値を持たない。
export function withRemoteSessionColor(preset: SettingsPreset, color: string | null): SettingsPreset {
  const appearance = { ...preset.appearance };
  delete appearance.remoteSessionColor;
  if (color !== null) appearance.remoteSessionColor = color;
  return { ...preset, appearance };
}

// remoteSessionFrameColor は、リモートセッションの枠と名札の色。その sshc エンジンの接続先（このマシンの
// Connections）に付けた色を先に使い、無ければプリセットの色。どちらも無ければ null で、さし色のまま。
export function remoteSessionFrameColor(connectionColour: string | undefined, presetColor: string | null): string | null {
  return (connectionColour === undefined ? null : parseAccentColor(connectionColour)) ?? presetColor;
}
