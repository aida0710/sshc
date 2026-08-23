plugins { id("com.android.application") }

// リリース時はタグ由来のバージョンを渡す。ローカルビルドでは既定値を使う。
val taggedVersionName = (findProperty("sshcVersionName") as String?) ?: "0.2.1"

// versionName と versionCode の不一致を防ぐため、番号は名前から導出する。
fun versionCodeOf(name: String): Int {
    val parsed = Regex("""^(\d+)\.(\d+)\.(\d+)""").find(name)
        ?: throw GradleException("sshcVersionName must start with major.minor.patch, got: $name")
    val (major, minor, patch) = parsed.destructured
    if (minor.toInt() > 999 || patch.toInt() > 999) {
        throw GradleException("minor and patch must stay under 1000 to keep the code ordered: $name")
    }
    return major.toInt() * 1_000_000 + minor.toInt() * 1_000 + patch.toInt()
}

// 署名情報は環境変数から受け取り、リポジトリには保存しない。リリース workflow
// では apksigner でも成果物を検証する。
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

    // Android API に依存しないネイティブ層の判断を JVM 上で検証する。
    testImplementation("junit:junit:4.13.2")
}
