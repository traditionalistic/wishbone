plugins {
    `java-library` // for the api(...) configuration below — :wishbone-ui needs kotlinx-serialization-json's
    // JsonObject/JsonElement types on its own compile classpath (Document.Node.raw, Document.decode(...)).
    alias(libs.plugins.kotlin.jvm)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    api(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.core)

    testImplementation(libs.junit5.api)
    testImplementation(libs.kotlinx.coroutines.test)
    testRuntimeOnly(libs.junit5.engine)
    testRuntimeOnly(libs.junit.platform.launcher)
}

tasks.test {
    useJUnitPlatform()
}
