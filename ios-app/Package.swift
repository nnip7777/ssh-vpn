// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SSHTunnel",
    platforms: [.iOS(.v15)],
    dependencies: [],
    targets: [
        .executableTarget(
            name: "SSHTunnel",
            path: "Sources/SSHTunnel"
        )
    ]
)
