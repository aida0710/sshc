package com.github.aida0710.sshc;

/** Serviceの一世代がprocess-global Go engineを所有しているかを直列化する。 */
final class EngineLease {
    private boolean owned;
    private boolean stopping;

    /**
     * Mobile.startの成功をmain looperへ渡す前に記録する。falseなら停止要求が
     * 先行しており、後のService世代が起動する前にworker自身がMobile.stopする。
     */
    synchronized boolean publishStartedEngine() {
        owned = true;
        if (!stopping) return true;
        owned = false;
        return false;
    }

    /** このService世代を停止中にし、Mobile.stopの実行権を一度だけ渡す。 */
    synchronized boolean requestStop() {
        stopping = true;
        if (!owned) return false;
        owned = false;
        return true;
    }

    synchronized boolean isStopping() {
        return stopping;
    }
}
