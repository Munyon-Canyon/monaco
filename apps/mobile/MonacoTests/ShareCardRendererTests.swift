import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ShareCardRendererTests {
    @Test func theCardIs1080By1350Pixels() throws {
        let image = try #require(ShareCardRenderer.render(ShareCardSamples.cabalGain, face: nil))

        #expect(image.scale == 3)
        #expect(image.cgImage?.width == 1080)
        #expect(image.cgImage?.height == 1350)
    }

    @Test func theMeCardIsTheSameSize() throws {
        let image = try #require(ShareCardRenderer.render(ShareCardSamples.me(.loss), face: nil))

        #expect(image.cgImage?.width == 1080)
        #expect(image.cgImage?.height == 1350)
    }

    @Test func aCardWithoutHistoryStillRendersAtFullSize() throws {
        for count in [0, 1] {
            let source = ShareCardSamples.cabalGain
            let content = ShareCardContent(
                kind: source.kind, title: source.title, subtitle: source.subtitle, status: source.status,
                dateText: source.dateText, points: Array(source.points.prefix(count)), direction: source.direction,
                gain: source.gain, returnText: source.returnText, stats: source.stats, footerLink: source.footerLink,
                shareURL: source.shareURL, spoken: source.spoken, faceSeed: source.faceSeed, faceName: source.faceName,
                faceURL: nil)

            let image = try #require(ShareCardRenderer.render(content, face: nil))

            #expect(image.cgImage?.width == 1080)
            #expect(image.cgImage?.height == 1350)
        }
    }

    @Test func theCanvasIsTheIdentityGreen() {
        let canvas = WCAGContrast.resolve(MonacoTheme.ShareCard.canvas, .dark)

        #expect(abs(canvas.red - 12.0 / 255) < 0.005)
        #expect(abs(canvas.green - 56.0 / 255) < 0.005)
        #expect(abs(canvas.blue - 40.0 / 255) < 0.005)
    }

    @Test func everyShareCardLabelClearsAAOnTheCanvasAndThePanel() {
        let card = MonacoTheme.ShareCard.self
        for background in [card.canvas, card.surface] {
            for label in [card.ink, card.muted, card.profit, card.loss, card.brand] {
                #expect(WCAGContrast.ratio(label, on: [background], .dark) >= 4.5)
            }
        }
    }

    @Test func aGainCurveEndsOnAProfitDot() throws {
        let points = ShareCardSamples.me(.gain).points

        let pixel = try lastPointPixel(points, tint: MonacoTheme.ShareCard.profit)

        #expect(distance(pixel, MonacoTheme.ShareCard.profit) < distance(pixel, MonacoTheme.ShareCard.canvas))
        #expect(distance(pixel, MonacoTheme.ShareCard.profit) < distance(pixel, MonacoTheme.ShareCard.loss))
    }

    @Test func aLossCurveEndsOnALossDot() throws {
        let points = ShareCardSamples.me(.loss).points

        let pixel = try lastPointPixel(points, tint: MonacoTheme.ShareCard.loss)

        #expect(distance(pixel, MonacoTheme.ShareCard.loss) < distance(pixel, MonacoTheme.ShareCard.canvas))
        #expect(distance(pixel, MonacoTheme.ShareCard.loss) < distance(pixel, MonacoTheme.ShareCard.profit))
    }

    @Test func theCurveKeepsEveryPointInsideItsFrame() {
        let size = CGSize(width: 300, height: 150)
        let points = ShareCardSamples.me(.gain).points

        for index in points.indices {
            let point = ShareCardCurve.plotPoint(for: index, of: points, in: size)
            #expect(point.x >= 0 && point.x <= size.width)
            #expect(point.y >= 0 && point.y <= size.height)
        }
    }

    @Test func theFaceFallbackUsesTheAppsTintForTheSeed() {
        #expect(ShareCardFace.tint(seed: "cabal-1") == MonacoTheme.CabalTint.forGroupId("cabal-1"))
        #expect(ShareCardFace.tint(seed: "CABAL-1") == MonacoTheme.CabalTint.forGroupId("cabal-1"))
    }

    private func lastPointPixel(_ points: [CurvePoint], tint: Color) throws -> (r: Double, g: Double, b: Double) {
        let size = CGSize(width: 300, height: 150)
        let view = ShareCardCurve(points: points, tint: tint)
            .frame(width: size.width, height: size.height)
            .background(MonacoTheme.ShareCard.canvas)
            .environment(\.colorScheme, .dark)
        let renderer = ImageRenderer(content: view)
        renderer.scale = 3
        let cgImage = try #require(renderer.uiImage?.cgImage)
        let spot = ShareCardCurve.plotPoint(for: points.count - 1, of: points, in: size)
        return try pixel(of: cgImage, x: Int(spot.x * 3), y: Int(spot.y * 3))
    }

    private func pixel(of image: CGImage, x: Int, y: Int) throws -> (r: Double, g: Double, b: Double) {
        var bytes = [UInt8](repeating: 0, count: 4)
        let context = try #require(
            CGContext(
                data: &bytes, width: 1, height: 1, bitsPerComponent: 8, bytesPerRow: 4,
                space: CGColorSpace(name: CGColorSpace.sRGB) ?? CGColorSpaceCreateDeviceRGB(),
                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        context.draw(image, in: CGRect(x: -x, y: -(image.height - 1 - y), width: image.width, height: image.height))
        return (Double(bytes[0]) / 255, Double(bytes[1]) / 255, Double(bytes[2]) / 255)
    }

    private func distance(_ pixel: (r: Double, g: Double, b: Double), _ color: Color) -> Double {
        var red: CGFloat = 0
        var green: CGFloat = 0
        var blue: CGFloat = 0
        UIColor(color).getRed(&red, green: &green, blue: &blue, alpha: nil)
        let (dr, dg, db) = (pixel.r - red, pixel.g - green, pixel.b - blue)
        return (dr * dr + dg * dg + db * db).squareRoot()
    }
}
