import ProjectDescription

let project = Project(
    name: "Playground",
    organizationName: "app.wishbone",
    packages: [
        .package(path: ".")
    ],
    settings: .settings(base: [
        "SWIFT_VERSION": "6.0"
    ]),
    targets: [
        .target(
            name: "Playground",
            destinations: [.iPhone],
            product: .app,
            bundleId: "app.wishbone.playground",
            deploymentTargets: .iOS("18.0"),
            infoPlist: .extendingDefault(with: [
                "UILaunchScreen": [:],
                "NSAppTransportSecurity": [
                    "NSAllowsArbitraryLoads": true,
                    "NSAllowsLocalNetworking": true
                ]
            ]),
            sources: ["Playground/Sources/**"],
            dependencies: [
                .package(product: "WishboneUI"),
                .package(product: "WishboneLottie"),
                .package(product: "WishboneRive")
            ]
        )
    ]
)
