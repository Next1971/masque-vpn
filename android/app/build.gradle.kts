import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Optional release signing. Create a keystore.properties file in the project
// root (NOT committed to git) with:
//   storeFile=/absolute/path/to/masque-release.keystore
//   storePassword=...
//   keyAlias=masque
//   keyPassword=...
// If the file is absent, the release build stays unsigned (still installable
// after enabling "unknown sources", but Play Store requires a signed build).
val keystorePropsFile = rootProject.file("keystore.properties")
val keystoreProps = Properties().apply {
    if (keystorePropsFile.exists()) keystorePropsFile.inputStream().use { load(it) }
}

android {
    namespace = "com.next1971.masque"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.next1971.masque"
        minSdk = 24          // Android 7.0 — CreateUnmonitoredTUNFromFD and VpnService are available
        targetSdk = 34
        versionCode = 1
        versionName = "1.0"
    }

    // Two form factors from one codebase:
    //   phone -> handset/tablet APK  (applicationId com.next1971.masque)
    //   tv    -> Android TV APK       (applicationId com.next1971.masque.tv)
    // Both share the same Go core (masque.aar), MasqueVpnService and ProfileStore.
    // The TV flavor adds a leanback launcher + D-pad UI + paste/text import,
    // and installs side by side with the phone build (distinct applicationId).
    flavorDimensions += "formFactor"
    productFlavors {
        create("phone") {
            dimension = "formFactor"
            // keeps applicationId = com.next1971.masque
        }
        create("tv") {
            dimension = "formFactor"
            applicationIdSuffix = ".tv"
            versionNameSuffix = "-tv"
            // app_name for TV is provided by src/tv/res/values/strings.xml
        }
    }

    signingConfigs {
        if (keystorePropsFile.exists()) {
            create("release") {
                storeFile = file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (keystorePropsFile.exists()) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    
}
kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}
dependencies {
    // Go core through gomobile. Place masque.aar in app/libs/ (see README).
    implementation(files("libs/masque.aar"))
    implementation("androidx.activity:activity-ktx:1.9.2")
    implementation("androidx.appcompat:appcompat:1.7.0")
    // Android TV (leanback) — used only by the tv flavor UI.
    "tvImplementation"("androidx.leanback:leanback:1.0.0")
}
