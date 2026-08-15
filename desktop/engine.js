"use strict";

// spawnEngine は、Electron が lifetime を所有する子 engine を起こす。
// `engine` をここで固定するのは、旧 flag を各 caller が手で渡す余地をなくし、
// parser が公開する owner kind と desktop の起動契約を同じ語に保つためである。
function spawnEngine(spawn, binary) {
  return spawn(binary, ["engine"], { stdio: ["ignore", "pipe", "inherit"] });
}

module.exports = { spawnEngine };
