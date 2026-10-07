import assert from "node:assert/strict";
import test from "node:test";
import { StartupProgress } from "../src/startup-progress.js";
import { machineDefinitions } from "../src/config.js";

const assetSizes = {
  "v86.wasm": 100, "seabios.bin": 10, "vgabios.bin": 10, "kernel.bin": 400,
  "client.cpio.gz": 480, "server.cpio.gz": 80,
};

test("WASMとBIOSが同じ番号でもファイルごとに読み込み割合を計算する", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.configureDownloads(assetSizes);
  progress.updateDownload("client", { file_index: 0, file_name: "./v86/v86.wasm", loaded: 25, total: 50 });
  assert.equal(progress.snapshot.machines[0].downloadPercentage, 5);
  assert.equal(progress.snapshot.downloadPercentage, 2);
  progress.updateDownload("client", { file_index: 0, file_name: "./v86/v86.wasm", loaded: 10, total: 50 });
  assert.equal(progress.snapshot.machines[0].downloadPercentage, 5);
  progress.updateDownload("client", { file_index: 0, file_name: "https://demo.test/images/seabios.bin", loaded: 10, total: 10 });
  assert.equal(progress.snapshot.machines[0].downloadPercentage, 11);
  assert.equal(progress.snapshot.phase, "download");
});

test("ファイル取得が完了してもVMとエンジンとWeb UIが揃うまで起動完了にしない", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.configureDownloads(assetSizes);
  for (const machine of machineDefinitions) progress.markMachineStarted(machine.role);
  assert.equal(progress.snapshot.downloadPercentage, 100);
  assert.equal(progress.snapshot.phase, "boot");
  progress.markMachineBooted("demo-a");
  progress.markMachineBooted("demo-a");
  assert.equal(progress.snapshot.bootedCount, 1);
  progress.markMachineBooted("demo-b");
  progress.markMachineBooted("client");
  assert.equal(progress.snapshot.phase, "engine");
  assert.equal(progress.snapshot.machines[0].state, "engine");
  progress.markEngineReady();
  assert.equal(progress.snapshot.phase, "web");
  progress.markUIReady();
  assert.equal(progress.snapshot.phase, "ready");
});

test("読み込み失敗後に遅れた準備完了イベントが来ても成功に変わらない", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.configureDownloads(assetSizes);
  progress.fail("demo-b");
  progress.markEngineReady();
  progress.markUIReady();
  for (const machine of machineDefinitions) {
    progress.markMachineStarted(machine.role);
    progress.markMachineBooted(machine.role);
  }
  assert.equal(progress.snapshot.phase, "failed");
  assert.equal(progress.snapshot.machines[2].state, "failed");
});

test("読み込みサイズがまだ不明な間は割合を捏造しない", () => {
  const progress = new StartupProgress(machineDefinitions);
  assert.equal(progress.snapshot.phase, "configuration");
  assert.equal(progress.snapshot.downloadPercentage, null);
});

test("UIの展開が完了するまでVMが起動済みでも読み込みを終えない", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.configureDownloads(assetSizes, { bytes: 1000 });
  for (const machine of machineDefinitions) {
    progress.markMachineStarted(machine.role);
    progress.markMachineBooted(machine.role);
  }
  progress.markEngineReady();
  progress.updateUIArchive({ state: "downloading", fraction: 0.5 });
  assert.equal(progress.snapshot.downloadPercentage, 84);
  progress.updateUIArchive({ state: "unpacking", fraction: 1 });
  assert.equal(progress.snapshot.downloadPercentage, 100);
  assert.equal(progress.snapshot.phase, "download");
  progress.updateUIArchive({ state: "ready", fraction: 1 });
  assert.equal(progress.snapshot.phase, "web");
});

test("GitHubのデモ一式を取得している間は読み込み段階としてその割合を出す", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.startReleaseArchive({ version: "v1.2.3", bytes: 1000 });
  assert.equal(progress.snapshot.phase, "download");
  assert.equal(progress.snapshot.downloadPercentage, 0);
  progress.updateReleaseArchive({ state: "downloading", fraction: 0.42 });
  assert.equal(progress.snapshot.downloadPercentage, 42);
  assert.deepEqual(progress.snapshot.releaseArchive,
    { version: "v1.2.3", state: "downloading", downloadPercentage: 42, completedFiles: undefined, totalFiles: undefined });
  progress.updateReleaseArchive({ state: "caching", fraction: 1, completedFiles: 3, totalFiles: 40 });
  assert.equal(progress.snapshot.phase, "download");
  assert.equal(progress.snapshot.releaseArchive.completedFiles, 3);
});

test("取得できなかったデモ一式は配信済みの版の読み込み割合に混ぜない", () => {
  const progress = new StartupProgress(machineDefinitions);
  progress.startReleaseArchive({ version: "v1.2.3", bytes: 1000 });
  progress.updateReleaseArchive({ state: "downloading", fraction: 0.5 });
  progress.markReleaseArchiveUnavailable();
  assert.equal(progress.snapshot.phase, "configuration");
  assert.equal(progress.snapshot.downloadPercentage, null);
  progress.configureDownloads(assetSizes);
  assert.equal(progress.snapshot.downloadPercentage, 0);
  assert.equal(progress.snapshot.releaseArchive.state, "unavailable");
});
