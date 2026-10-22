package com.github.aida0710.sshc;

import java.util.HashSet;
import java.util.Set;

/**
 * このアプリが出したターミナルの通知の tag を覚える。MainActivity は exported なので、
 * ほかのアプリも通知の tag の extra を付けて開ける。覚えている tag だけを、押された通知
 * として Web 画面へ渡す。
 *
 * プロセスの中にだけ持つ。ターミナルの接続も同じプロセスの sshc エンジンが持つので、
 * プロセスが終わればその tag の指す接続も無い。SharedPreferences などに残す必要は無い。
 */
final class IssuedNotificationTags {
    private final Set<String> tags = new HashSet<>();

    synchronized void issue(String tag) {
        tags.add(tag);
    }

    /** tag をこのアプリが出していれば、覚えた tag から外して true を返す。 */
    synchronized boolean take(String tag) {
        return tags.remove(tag);
    }
}
