import { chromium } from "@playwright/test";
import { fileURLToPath } from "node:url";

const output = fileURLToPath(
  new URL("../public/icon-maskable-512.png", import.meta.url),
);

const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({
    viewport: { width: 512, height: 512 },
    deviceScaleFactor: 1,
  });
  await page.setContent(`<!doctype html>
    <style>html, body { margin: 0; width: 512px; height: 512px; overflow: hidden; }</style>
    <svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
      <defs>
        <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stop-color="#38BDF8"/>
          <stop offset="58%" stop-color="#667EEA"/>
          <stop offset="100%" stop-color="#8B5CF6"/>
        </linearGradient>
      </defs>
      <rect width="512" height="512" fill="#09111F"/>
      <g fill="none" stroke="url(#g)" stroke-width="28" stroke-linejoin="round" stroke-linecap="round">
        <path d="M136 166H244L288 210H376V346H268L224 302H136Z"/>
      </g>
      <rect x="304" y="248" width="24" height="52" rx="3" fill="#EAF2FF"/>
    </svg>`);
  await page.screenshot({ path: output, type: "png" });
} finally {
  await browser.close();
}
