import { useEffect, useState } from "react";
import { apiClient } from "../api/client";


export async function backgroundImageURL(name: string): Promise<string> {
  const response = await apiClient.send(`/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`, {
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
