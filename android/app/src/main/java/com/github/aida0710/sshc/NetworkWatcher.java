package com.github.aida0710.sshc;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.Network;

import mobile.Mobile;

/**
 * 回線の有無をエンジンへ伝える。回線が無い間、切れたセッションは再接続の回数を使わずに待ち、
 * 回線があると伝えるとすぐ試す。Wi-Fi とモバイル回線の切り替えでも onAvailable が来る。
 */
final class NetworkWatcher {
    private final ConnectivityManager connectivity;
    private final ConnectivityManager.NetworkCallback callback = new ConnectivityManager.NetworkCallback() {
        @Override
        public void onAvailable(Network network) {
            Mobile.setNetworkAvailable(true);
        }

        @Override
        public void onLost(Network network) {
            Mobile.setNetworkAvailable(false);
        }
    };

    NetworkWatcher(Context context) {
        connectivity = context.getSystemService(ConnectivityManager.class);
    }

    /** 回線が無いときは onAvailable が来ないので、いまの状態を先に伝える。 */
    void start() {
        Mobile.setNetworkAvailable(connectivity.getActiveNetwork() != null);
        connectivity.registerDefaultNetworkCallback(callback);
    }

    void stop() {
        connectivity.unregisterNetworkCallback(callback);
    }
}
