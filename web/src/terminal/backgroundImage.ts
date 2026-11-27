import { useEffect, useState } from "react";
import { apiClient } from "../api/client";
import { engineApiBase } from "../api/remoteEngines";
import { useEngineApis } from "../shell/engineApis";

async function backgroundImageURL(base: string, name: string): Promise<string> {
  const response = await apiClient.send(`${base}/terminal/backgrounds/${encodeURIComponent(name)}`, {
    method: "GET",
  });
  if (!response.ok) throw new Error("background_unreadable");
  const bytes = await response.blob();
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      typeof reader.result === "string" ? resolve(reader.result) : reject(new Error("background_unreadable"));
    reader.onerror = () => reject(new Error("background_unreadable"));
    reader.readAsDataURL(bytes);
  });
}

// useBackgroundImage は、背景の画像 name を描ける URL にする。画像は、それを選んだ設定のエンジン（選んでいる
// リモートセッション）のライブラリから読む。
export function useBackgroundImage(name: string): string {
  const [url, setURL] = useState("");
  const base = engineApiBase(useEngineApis().runTarget);
  useEffect(() => {
    if (name === "") {
      setURL("");
      return;
    }
    let live = true;
    void backgroundImageURL(base, name)
      .then((next) => {
        if (live) setURL(next);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [base, name]);
  return url;
}
