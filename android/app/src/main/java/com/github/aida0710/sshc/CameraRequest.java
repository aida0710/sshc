package com.github.aida0710.sshc;

/**
 * Web 画面からのカメラの要求のうち、何を許すかの判断。Android API に依存しないため JVM テストで検証できる。
 *
 * カメラを使うのは、同期の設定の QR コードを読み取るときだけである。音声や保護されたメディアは
 * 画面のどこも使わないので、映像だけの要求のほかは断る。
 */
final class CameraRequest {
    /** android.webkit.PermissionRequest.RESOURCE_VIDEO_CAPTURE と同じ値。 */
    static final String VIDEO_CAPTURE = "android.webkit.resource.VIDEO_CAPTURE";

    private CameraRequest() {}

    /** 要求が映像（カメラ）だけか。 */
    static boolean asksOnlyForVideo(String[] resources) {
        return resources != null && resources.length == 1 && VIDEO_CAPTURE.equals(resources[0]);
    }
}
