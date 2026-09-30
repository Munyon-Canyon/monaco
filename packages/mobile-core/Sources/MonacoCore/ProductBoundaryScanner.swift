import Foundation

/// Forbidden external hosts for product flows — mobile talks to backend only.
public enum ProductBoundaryScanner {
    public static let forbiddenHostFragments = [
        "api.xstocks.fi",
        "jup.ag",
        "hermes.pyth.network",
        "pyth.network",
        "mainnet-beta.solana.com",
        "solana-mainnet",
    ]

    public static func containsForbiddenHost(_ text: String) -> Bool {
        let lowered = text.lowercased()
        return forbiddenHostFragments.contains { lowered.contains($0) }
    }

    /// Every regular file under `directory`, sorted by path. Walks with `FileManager` so it runs the
    /// same on macOS and Linux.
    public static func sourceFiles(under directory: URL) -> [URL] {
        guard let walk = FileManager.default.enumerator(at: directory, includingPropertiesForKeys: [.isRegularFileKey])
        else {
            return []
        }
        return
            walk
            .compactMap { $0 as? URL }
            .filter { (try? $0.resourceValues(forKeys: [.isRegularFileKey]).isRegularFile) == true }
            .sorted { $0.path < $1.path }
    }

    public static func featureSourcesAreClean(under directory: URL) throws -> Bool {
        try sourceFiles(under: directory)
            .filter { $0.pathExtension == "swift" }
            .allSatisfy { try !containsForbiddenHost(String(contentsOf: $0, encoding: .utf8)) }
    }
}
