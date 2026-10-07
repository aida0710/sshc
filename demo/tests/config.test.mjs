import assert from "node:assert/strict";
import test from "node:test";
import { bundleDownloadBytes } from "../src/config.js";

test("配信済みの版のダウンロード量は起動ファイルとWeb UIを1回ずつ数える", () => {
  const assetSizes = { "v86.wasm": 100, "kernel.bin": 400, "client.cpio.gz": 480, "server.cpio.gz": 80 };
  assert.equal(bundleDownloadBytes({ assetSizes, uiArchive: { bytes: 40 } }), 1100);
  assert.equal(bundleDownloadBytes({ assetSizes, uiArchive: undefined }), 1060);
});
