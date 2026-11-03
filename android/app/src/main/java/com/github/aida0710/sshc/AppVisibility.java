package com.github.aida0710.sshc;

/**
 * 利用者にアプリの画面が見えているか。通知はプロセスのサービスから出すので、Activity の外から読む。
 * Activity はプロセスに 1 つ（singleTask）なので、プロセスで 1 つ持てばよい。
 */
final class AppVisibility {
    private static volatile boolean visible;

    private AppVisibility() {}

    static void set(boolean nowVisible) {
        visible = nowVisible;
    }

    static boolean isVisible() {
        return visible;
    }
}
