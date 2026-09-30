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

    // release の APK は Gradle では署名しない。.github/workflows/release.yml が未署名の
    // APK を作り、stage-release の job の apksigner で署名と検証をする。
    buildTypes {
        getByName("debug") {
            applicationIdSuffix = ".dev"
            versionNameSuffix = "-dev"
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
    // Android 16 の predictive back と旧 OS の戻る操作を同じ callback で扱う。
    implementation("androidx.activity:activity:1.13.0")

    // Android API に依存しないネイティブ層の判断を JVM 上で検証する。
    testImplementation("junit:junit:4.13.2")
}
