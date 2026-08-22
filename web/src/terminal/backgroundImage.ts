import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

// 背景の画像を、CSS と <img> が読める綴りにして返す。
//
// **url() や <img src> では取りに行けない。** この API は読み取りにも CSRF
// トークンを要求しており、ブラウザが自分で出す要求にそれは付けられない。
// 要求している理由は cookie がポートに紐づかないことで（security.go）、
// 127.0.0.1 の別のサーバーが cookie を受け取りうる以上、そこは曲げられない
// ——**cookie 単体を無価値に保つことが、あの規則の目的そのものである。**
//
// だから JS が取りに行き、data: にして渡す。CSP の img-src は data: を許して
// いるので、ここだけで閉じる。画像は 1MiB を超えないので、抱えても軽い。

export async function backgroundImageURL(name: string): Promise<string> {
  const response = await apiClient.send(`/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`, {
    method: "GET",
  });
  if (!response.ok) throw new Error("background_unreadable");
  const bytes = await response.blob();
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    // readAsDataURL なら結果は必ず綴りだが、**型はそれを知らない。**
    // ArrayBuffer を String に通すと "[object ArrayBuffer]" が背景になる。
    reader.onload = () =>
      typeof reader.result === "string" ? resolve(reader.result) : reject(new Error("background_unreadable"));
    reader.onerror = () => reject(new Error("background_unreadable"));
    reader.readAsDataURL(bytes);
  });
}

/**
 * useBackgroundImage は、名前から綴りを引く。まだ読めていなければ空である。
 *
 * <p>**読めなかったことを画面に出さない。** 背景が出ないだけで端末は使える
 * ——画像ひとつのために、繋がっている端末の上へ警告を積む理由はない。
 */
export function useBackgroundImage(name: string): string {
  const [url, setURL] = useState("");
  useEffect(() => {
    if (name === "") {
      setURL("");
      return;
    }
    let live = true;
    void backgroundImageURL(name)
      .then((next) => {
        if (live) setURL(next);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [name]);
  return url;
}
