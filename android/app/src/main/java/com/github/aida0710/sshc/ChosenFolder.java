package com.github.aida0710.sshc;

import android.content.ContentResolver;
import android.database.Cursor;
import android.net.Uri;
import android.provider.DocumentsContract;
import android.webkit.WebResourceResponse;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.FileNotFoundException;
import java.io.InputStream;
import java.security.SecureRandom;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/**
 * Android のフォルダの選択画面で選んだフォルダの中身を一覧にし、各ファイルの中身を Web 画面へ渡す。
 * WebView の input はフォルダを選ばせないため、Web 画面はこの一覧を受け取り、各ファイルを
 * PATH_PREFIX の URL から読んで、ブラウザで選んだフォルダと同じようにアップロードする。
 */
final class ChosenFolder {
    /** ファイルの中身を渡す URL のパス。engine には届かず、WebView の shouldInterceptRequest が答える。 */
    static final String PATH_PREFIX = "/__sshc-android-folder/";
    // 一覧にするファイルの数と深さの上限。選んだフォルダが大きすぎても端末を止めない。
    // 転送キューは一度に 200 件までなので、それを超えた分は Web 画面がまとめて断る。
    static final int MAX_FILES = 10000;
    static final int MAX_DEPTH = 64;

    static final class TooManyFiles extends Exception {}

    private static final String[] COLUMNS = {
        DocumentsContract.Document.COLUMN_DOCUMENT_ID,
        DocumentsContract.Document.COLUMN_DISPLAY_NAME,
        DocumentsContract.Document.COLUMN_MIME_TYPE,
        DocumentsContract.Document.COLUMN_SIZE,
        DocumentsContract.Document.COLUMN_LAST_MODIFIED,
    };

    private final ContentResolver resolver;
    private final SecureRandom random = new SecureRandom();
    // URL の token から、選んだフォルダの中のファイルの document URI を引く。選び直すと入れ替わる。
    private final Map<String, Uri> files = Collections.synchronizedMap(new HashMap<>());

    ChosenFolder(ContentResolver resolver) {
        this.resolver = resolver;
    }

    /** 選んだフォルダの一覧を JSON で返す。前に選んだフォルダのファイルは渡さなくなる。 */
    String list(Uri tree) throws TooManyFiles, JSONException {
        files.clear();
        String rootId = DocumentsContract.getTreeDocumentId(tree);
        String rootName = displayName(DocumentsContract.buildDocumentUriUsingTree(tree, rootId));
        String root = childPath("", rootName == null ? "folder" : rootName);
        if (root == null) root = "folder";
        JSONArray directories = new JSONArray().put(root);
        JSONArray entries = new JSONArray();
        walk(tree, rootId, root, 1, directories, entries);
        return new JSONObject().put("directories", directories).put("files", entries).toString();
    }

    private void walk(Uri tree, String parentId, String parentPath, int depth, JSONArray directories, JSONArray entries)
            throws TooManyFiles, JSONException {
        if (depth > MAX_DEPTH) return;
        Uri children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, parentId);
        try (Cursor cursor = resolver.query(children, COLUMNS, null, null, null)) {
            if (cursor == null) return;
            while (cursor.moveToNext()) {
                String path = childPath(parentPath, cursor.getString(1));
                if (path == null) continue;
                String id = cursor.getString(0);
                if (DocumentsContract.Document.MIME_TYPE_DIR.equals(cursor.getString(2))) {
                    directories.put(path);
                    walk(tree, id, path, depth + 1, directories, entries);
                    continue;
                }
                if (files.size() >= MAX_FILES) throw new TooManyFiles();
                String token = newToken();
                files.put(token, DocumentsContract.buildDocumentUriUsingTree(tree, id));
                entries.put(new JSONObject()
                        .put("path", path)
                        .put("url", PATH_PREFIX + token)
                        .put("size", cursor.isNull(3) ? 0 : cursor.getLong(3))
                        .put("lastModified", cursor.isNull(4) ? 0 : cursor.getLong(4)));
            }
        }
    }

    /** PATH_PREFIX の URL なら、そのファイルの中身で答える。ほかの URL なら null。 */
    WebResourceResponse open(Uri url) {
        String path = url.getPath();
        if (path == null || !path.startsWith(PATH_PREFIX)) return null;
        Uri document = files.get(path.substring(PATH_PREFIX.length()));
        Map<String, String> headers = Collections.singletonMap("Cache-Control", "no-store");
        if (document == null) {
            return new WebResourceResponse("text/plain", "utf-8", 404, "Not Found", headers, null);
        }
        try {
            InputStream content = resolver.openInputStream(document);
            return new WebResourceResponse("application/octet-stream", null, 200, "OK", headers, content);
        } catch (FileNotFoundException | SecurityException error) {
            return new WebResourceResponse("text/plain", "utf-8", 404, "Not Found", headers, null);
        }
    }

    /** アップロードが済んだら、選んだフォルダのファイルを渡さなくする。 */
    void release() {
        files.clear();
    }

    private String displayName(Uri document) {
        try (Cursor cursor = resolver.query(document, new String[]{DocumentsContract.Document.COLUMN_DISPLAY_NAME}, null, null, null)) {
            return cursor != null && cursor.moveToFirst() ? cursor.getString(0) : null;
        }
    }

    private String newToken() {
        byte[] bytes = new byte[18];
        random.nextBytes(bytes);
        StringBuilder token = new StringBuilder(bytes.length * 2);
        for (byte value : bytes) token.append(String.format("%02x", value));
        return token.toString();
    }

    /** フォルダの中の 1 つの名前を、上の階層のパスへつなぐ。パスとして安全でない名前なら null。 */
    static String childPath(String parent, String name) {
        if (name == null || name.isEmpty() || name.equals(".") || name.equals("..")) return null;
        if (name.contains("/") || name.contains("\\") || name.matches(".*\\p{Cntrl}.*")) return null;
        return parent.isEmpty() ? name : parent + "/" + name;
    }
}
