import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct SceneColorTests {
    private func rgba(_ color: Color) -> [CGFloat] {
        var r: CGFloat = 0
        var g: CGFloat = 0
        var b: CGFloat = 0
        var a: CGFloat = 0
        UIColor(color).getRed(&r, green: &g, blue: &b, alpha: &a)
        return [r, g, b, a]
    }

    private func luminance(_ c: [CGFloat]) -> Double {
        let lin = c.prefix(3).map { v -> Double in
            let d = Double(v)
            return d <= 0.03928 ? d / 12.92 : pow((d + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]
    }

    private func lightness(_ color: Color) -> Double {
        let c = rgba(color)
        return Double(max(c[0], c[1], c[2]) + min(c[0], c[1], c[2])) / 2
    }

    private func solid(_ white: CGFloat) -> UIImage {
        UIGraphicsImageRenderer(size: CGSize(width: 8, height: 8)).image { context in
            UIColor(white: white, alpha: 1).setFill()
            context.fill(CGRect(x: 0, y: 0, width: 8, height: 8))
        }
    }

    @Test func firstSceneIsHarbourBlue() {
        let c = rgba(SceneColor.color(forScene: MonacoWorldScene.all[0]))
        let want: [CGFloat] = [14, 42, 61]
        for i in 0..<3 { #expect(abs(c[i] - want[i] / 255) < 0.5 / 255) }
        #expect(MonacoWorldScene.all.count == 26)
    }

    @Test func everySceneKeepsWhiteTextReadable() {
        for scene in MonacoWorldScene.all {
            let c = rgba(SceneColor.color(forScene: scene))
            let bg = luminance(c)
            #expect(1.05 / (bg + 0.05) >= 4.5)
            let blended = c.prefix(3).map { 0.7 + 0.3 * $0 }
            #expect((luminance(blended) + 0.05) / (bg + 0.05) >= 4.5)
        }
    }

    @Test func photoAverageIsClampedDark() {
        for white in [1, 0.5, 0] as [CGFloat] {
            #expect(lightness(SceneColor.color(forPhoto: solid(white))) <= 0.14 + 0.01)
        }
    }

    @Test func undecodableImageFallsBackToHarbour() {
        #expect(rgba(SceneColor.color(forPhoto: UIImage())) == rgba(Color(hex: 0x0E2A3D)))
    }

    @Test func resolveWithoutPictureIsTheSeedScene() async {
        let a = await SceneColor.resolve(cabalID: "cabal-1", pictureURL: nil)
        let b = await SceneColor.resolve(cabalID: "cabal-1", pictureURL: nil)
        #expect(rgba(a) == rgba(b))
        #expect(rgba(a) == rgba(SceneColor.color(forScene: .forSeed("cabal-1"))))
    }

    @Test func resolveFallsBackWhenLoadFails() async {
        let missing = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".png")
        let c = await SceneColor.resolve(cabalID: "cabal-2", pictureURL: missing)
        #expect(rgba(c) == rgba(SceneColor.color(forScene: .forSeed("cabal-2"))))
    }

    @Test func resolveFallsBackOnUndecodableDataAndCachesGoodPhotos() async throws {
        let dir = FileManager.default.temporaryDirectory
        let junk = dir.appendingPathComponent(UUID().uuidString + ".png")
        try Data("nope".utf8).write(to: junk)
        let bad = await SceneColor.resolve(cabalID: "cabal-3", pictureURL: junk)
        #expect(rgba(bad) == rgba(SceneColor.color(forScene: .forSeed("cabal-3"))))

        let good = dir.appendingPathComponent(UUID().uuidString + ".png")
        try #require(solid(0.5).pngData()).write(to: good)
        let first = await SceneColor.resolve(cabalID: "cabal-3", pictureURL: good)
        try FileManager.default.removeItem(at: good)
        let second = await SceneColor.resolve(cabalID: "cabal-3", pictureURL: good)
        #expect(rgba(first) == rgba(second))
        #expect(lightness(first) <= 0.15)
    }
}
