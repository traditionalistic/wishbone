pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "wishbone"

include(":wishbone-core")
include(":wishbone-ui")
include(":wishbone-lottie")
include(":wishbone-rive")
include(":playground")
