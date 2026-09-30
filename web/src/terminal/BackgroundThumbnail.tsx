import { useBackgroundImage } from "./backgroundImage";

export function BackgroundThumbnail({ name, chosen, className = "h-16 w-24 rounded-md" }: { name: string; chosen: boolean; className?: string }) {
  const url = useBackgroundImage(name);
  if (url === "") return <div className={`${className} bg-control`} />;
  return <img src={url} alt={name} className={`${className} object-cover ${chosen ? "brightness-105" : ""}`} />;
}
