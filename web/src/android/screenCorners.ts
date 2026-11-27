import { useEffect, useState } from "react";

// CornerReach は、画面の角の丸みが、ページの1つの角にどれだけ入り込んでいるか（CSS px）。horizontal は
// ページの左右の辺から、vertical は上下の辺から、丸みの中心までの長さ。この2つを半径にした楕円は、
// 角の丸みの内側に収まる（android の ScreenCorners）。[0, 0] は入り込んでいない角。
export type CornerReach = readonly [horizontal: number, vertical: number];

export type ScreenCorners = {
  topLeft: CornerReach;
  topRight: CornerReach;
  bottomRight: CornerReach;
  bottomLeft: CornerReach;
};

function isLength(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function cornerReachOf(value: unknown): CornerReach | null {
  if (!Array.isArray(value) || value.length !== 2) return null;
  const [horizontal, vertical] = value as unknown[];
  return isLength(horizontal) && isLength(vertical) ? [horizontal, vertical] : null;
}

// parseScreenCorners は、Android のアプリが渡す JSON を読む。形が違えば null。
export function parseScreenCorners(json: string): ScreenCorners | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(json);
  } catch {
    return null;
  }
  if (typeof parsed !== "object" || parsed === null) return null;
  const record = parsed as Record<string, unknown>;
  const topLeft = cornerReachOf(record.topLeft);
  const topRight = cornerReachOf(record.topRight);
  const bottomRight = cornerReachOf(record.bottomRight);
  const bottomLeft = cornerReachOf(record.bottomLeft);
  if (topLeft === null || topRight === null || bottomRight === null || bottomLeft === null) return null;
  return { topLeft, topRight, bottomRight, bottomLeft };
}

function readScreenCorners(target: Window): ScreenCorners | null {
  const bridge = target.sshcAndroid;
  if (bridge === undefined) return null;
  try {
    return parseScreenCorners(bridge.screenCorners());
  } catch {
    // renderer の再起動と同時に bridge が消えた。角は丸めずに描く。
    return null;
  }
}

// useScreenCorners は、Android のアプリで、画面の角の丸みがページの四隅にどれだけ入り込んでいるかを返す。
// 回転やキーボードの開閉で変わると、アプリが sshc-android-screen-corners で知らせる。ブラウザでは null。
export function useScreenCorners(target: Window = window): ScreenCorners | null {
  const [corners, setCorners] = useState(() => readScreenCorners(target));
  useEffect(() => {
    if (target.sshcAndroid === undefined) return;
    const reread = () => setCorners(readScreenCorners(target));
    reread();
    target.addEventListener("sshc-android-screen-corners", reread);
    return () => target.removeEventListener("sshc-android-screen-corners", reread);
  }, [target]);
  return corners;
}
