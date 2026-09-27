// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Wishbone",
    platforms: [
        .iOS(.v18),
        .macOS(.v15)
    ],
    products: [
        .library(name: "WishboneCore", targets: ["WishboneCore"]),
        .library(name: "WishboneUI", targets: ["WishboneUI"]),
        .library(name: "WishboneLottie", targets: ["WishboneLottie"]),
        .library(name: "WishboneRive", targets: ["WishboneRive"])
    ],
    dependencies: [
        .package(url: "https://github.com/airbnb/lottie-spm.git", from: "4.6.1"),
        .package(url: "https://github.com/rive-app/rive-ios.git", from: "6.27.0")
    ],
    targets: [
        .target(
            name: "WishboneCore"
        ),
        .target(
            name: "WishboneUI",
            dependencies: ["WishboneCore"],
            resources: [.copy("Resources/korpu.json"), .copy("Resources/Fonts")]
        ),
        .target(
            name: "WishboneLottie",
            dependencies: [
                "WishboneUI",
                .product(name: "Lottie", package: "lottie-spm")
            ]
        ),
        .target(
            name: "WishboneRive",
            dependencies: [
                "WishboneUI",
                .product(name: "RiveRuntime", package: "rive-ios")
            ]
        ),
        .testTarget(
            name: "WishboneCoreTests",
            dependencies: ["WishboneCore"]
        ),
        .testTarget(
            name: "WishboneUITests",
            dependencies: ["WishboneUI"]
        )
    ],
    swiftLanguageModes: [.v6]
)
