// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "call-analyzer-capture",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .executable(
            name: "call-analyzer-capture",
            targets: ["call-analyzer-capture"]
        )
    ],
    targets: [
        .executableTarget(
            name: "call-analyzer-capture",
            path: "Sources/call-analyzer-capture",
            exclude: ["Info.plist"],
            linkerSettings: [
                .unsafeFlags([
                    "-Xlinker", "-sectcreate",
                    "-Xlinker", "__TEXT",
                    "-Xlinker", "__info_plist",
                    "-Xlinker", "Sources/call-analyzer-capture/Info.plist",
                ])
            ]
        ),
        .testTarget(
            name: "call-analyzer-captureTests",
            dependencies: ["call-analyzer-capture"],
            path: "Tests/call-analyzer-captureTests"
        )
    ]
)
