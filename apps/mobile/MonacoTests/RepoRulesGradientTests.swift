import Foundation
import Testing

struct RepoRulesGradientTests {
    @Test
    func noSourceDrawsAGradient() throws {
        let sources = try Self.sources()
        #expect(sources.count > 100, "scanned \(sources.count) files from \(#filePath)")
        let flagged = sources.flatMap { path, source in
            GradientRule.violations(in: source).map { "\(path):\($0)" }
        }
        #expect(flagged.isEmpty, "the app draws flat colour only, no gradients: \(flagged)")
    }

    @Test
    func theRuleFlagsEveryGradientForm() {
        for source in [
            "LinearGradient(colors: [], startPoint: .top, endPoint: .bottom)",
            "RadialGradient(colors: [], center: .center, startRadius: 0, endRadius: 1)",
            "AngularGradient(colors: [], center: .center)",
            "EllipticalGradient(colors: [])",
            "MeshGradient(width: 2, height: 2, points: [], colors: [])",
            "context.fill(path, with: .linearGradient(gradient, startPoint: a, endPoint: b))",
            "let g = Gradient(colors: [])",
        ] {
            #expect(GradientRule.violations(in: source) == [1], "missed: \(source)")
        }
    }

    @Test
    func theRulePassesFlatColour() {
        #expect(GradientRule.violations(in: "Color.clear.overlay { MonacoTheme.coverOverlay }\n// no gradient") == [])
    }

    private static func repoRoot(filePath: String) -> URL? {
        let fromSource = URL(filePath: filePath).deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let products = ProcessInfo.processInfo.environment["__XCODE_BUILT_PRODUCTS_DIR_PATHS"] ?? ""
        let fromProducts = products.range(of: "/.build/").map { URL(filePath: String(products[..<$0.lowerBound])) }
        return [fromSource, fromProducts].compactMap { $0 }.first {
            FileManager.default.fileExists(atPath: $0.appending(path: "apps/mobile/Monaco").path)
        }
    }

    private static func sources(filePath: String = #filePath) throws -> [(String, String)] {
        let repo = try #require(repoRoot(filePath: filePath), "cannot find the repo from \(filePath)")
        let roots = [repo.appending(path: "apps/mobile/Monaco"), repo.appending(path: "packages/mobile-core/Sources")]
        return try roots.flatMap { root -> [(String, String)] in
            let files = try #require(FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil))
            return try files.compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }.map { url in
                (url.standardizedFileURL.path, try String(contentsOf: url, encoding: .utf8))
            }
        }
    }
}

enum GradientRule {
    static func violations(in source: String) -> [Int] {
        let pattern = #"(Linear|Radial|Angular|Elliptical|Mesh)Gradient|\.linearGradient\(|\bGradient\("#
        return source.split(separator: "\n", omittingEmptySubsequences: false).enumerated().compactMap { index, line in
            line.range(of: pattern, options: .regularExpression) == nil ? nil : index + 1
        }
    }
}
