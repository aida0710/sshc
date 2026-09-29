// VPN画面が経路の状態を読み直す間隔。どちらも読み直すたびに sshcエンジンが docker を
// 1回呼ぶので、人が変化に気づける程度に留める。

// routeProgressIntervalMs は、経路を用意しているあいだの間隔である。用意は分単位に
// なることがあり、どの段階まで進んだかを見せる。
export const routeProgressIntervalMs = 2000;

// overviewRefreshIntervalMs は、それ以外のときの間隔である。経路はこの画面の外でも
// 変わる（ほかの画面や CLI から開いた接続が経路を起動する、誰も使わなくなった経路を
// sshcエンジンが停止する）。開き直さなくても、いまの状態で「接続」と「切断」を押せる
// ようにする。
export const overviewRefreshIntervalMs = 5000;
