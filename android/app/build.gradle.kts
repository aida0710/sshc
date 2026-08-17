plugins { id("com.android.application") }

android {
    namespace = "com.github.aida0710.sshc"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.github.aida0710.sshc"
        minSdk = 26
        targetSdk = 36
        versionCode = 11
        versionName = "0.2.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    // sshc.aar は `make android-bind` が置く。Go の成果物なので追跡しない。
    implementation(files("libs/sshc.aar"))
}
