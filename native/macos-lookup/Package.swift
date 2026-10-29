// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "call-analyzer-lookup",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .executable(
            name: "call-analyzer-lookup",
            targets: ["call-analyzer-lookup"]
        )
    ],
    targets: [
        .executableTarget(
            name: "call-analyzer-lookup",
            path: "Sources/call-analyzer-lookup",
            exclude: ["Info.plist"],
            linkerSettings: [
                .unsafeFlags([
                    "-Xlinker", "-sectcreate",
                    "-Xlinker", "__TEXT",
                    "-Xlinker", "__info_plist",
                    "-Xlinker", "Sources/call-analyzer-lookup/Info.plist",
                ])
            ]
        ),
        .testTarget(
            name: "call-analyzer-lookupTests",
            dependencies: ["call-analyzer-lookup"],
            path: "Tests/call-analyzer-lookupTests"
        ),
    ]
)
