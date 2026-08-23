plugins { id("com.android.application") }

// **版はタグが決める。** 手で書いた数のまま配ると、タグを打つたびに同じ
// versionCode の APK が出る——Android は versionCode が増えない限り更新を
// 受け付けないので、二度目以降のリリースが利用者に届かない。しかも黙って
// 届かない。デスクトップ側は desktop-version がタグから版を入れており、
// ここだけが取り残されていた。
//
// 手元のビルドは既定のままでよい。渡されたときだけ上書きする。
val taggedVersionName = (findProperty("sshcVersionName") as String?) ?: "0.2.1"

// **番号は名前から導く。二つを別々に渡さない。** 別々にすると、名前だけ上げて
// 番号を忘れたリリースが作れてしまい、それは「新しいのに更新されない APK」
// として利用者の側にだけ現れる。入力をタグひとつに保てば、その組み合わせは
// 存在しなくなる。
fun versionCodeOf(name: String): Int {
    val parsed = Regex("""^(\d+)\.(\d+)\.(\d+)""").find(name)
        ?: throw GradleException("sshcVersionName must start with major.minor.patch, got: $name")
    val (major, minor, patch) = parsed.destructured
    if (minor.toInt() > 999 || patch.toInt() > 999) {
        throw GradleException("minor and patch must stay under 1000 to keep the code ordered: $name")
    }
    return major.toInt() * 1_000_000 + minor.toInt() * 1_000 + patch.toInt()
}

// **署名の材料は環境から取る。** 鍵をリポジトリに置かないためである。
// 無いときにここで組み立てないのは、**未署名の APK を「出来た」と言わない**
// ためでもある。署名されていない APK は配布物ではない——インストールすら
// できない。出来たかどうかは、リリースの側が apksigner で確かめる。
val keystorePath: String? = System.getenv("ANDROID_KEYSTORE_PATH")

android {
    namespace = "com.github.aida0710.sshc"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.github.aida0710.sshc"
        minSdk = 26
        targetSdk = 36
        versionCode = versionCodeOf(taggedVersionName)
        versionName = taggedVersionName
    }

    if (keystorePath != null) {
        signingConfigs {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
        buildTypes {
            getByName("release") {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    // sshc.aar は `make android-bind` が置く。Go の成果物なので追跡しない。
    implementation(files("libs/sshc.aar"))

    // **端末もエミュレータも要らない検査のためだけに在る。** 外殻の判断のうち
    // Android を必要としないものは src/test へ出してあり、そこは JVM で走る。
    // ここに androidTest（実機が要る側）は無い——CI が持てないものを置くと、
    // 「在るのに一度も走らない検査」になる。
    testImplementation("junit:junit:4.13.2")
}
