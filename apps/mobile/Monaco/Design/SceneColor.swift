import CoreImage
import SwiftUI
import UIKit

nonisolated enum SceneColor {
    private static let fallback: UInt32 = 0x0E2A3D
    private static let maxLightness: CGFloat = 0.14
    private static let maxSaturation: CGFloat = 0.45

    private static let table: [UInt32] = [
        0x0E2A3D, 0x2E221A, 0x2E231A, 0x2E2119, 0x2E2119, 0x2D241A, 0x232126,
        0x28241F, 0x2F2219, 0x302017, 0x2F2118, 0x282020, 0x2A211D, 0x2D221A,
        0x2B221C, 0x322416, 0x2F2418, 0x2D201A, 0x262223, 0x2B201D, 0x2C231B,
        0x302418, 0x2F2118, 0x2F2418, 0x2E2119, 0x282120,
    ]

    static func color(forScene scene: MonacoWorldScene) -> Color {
        Color(uiColor: UIColor(hex: table[scene.index]))
    }

    static func color(forPhoto image: UIImage) -> Color {
        guard let input = CIImage(image: image) else { return Color(uiColor: UIColor(hex: fallback)) }
        let extent = CIVector(cgRect: input.extent)
        let average = input.applyingFilter("CIAreaAverage", parameters: [kCIInputExtentKey: extent])
        var pixel = [UInt8](repeating: 0, count: 4)
        CIContext().render(
            average, toBitmap: &pixel, rowBytes: 4, bounds: CGRect(x: 0, y: 0, width: 1, height: 1),
            format: .RGBA8, colorSpace: CGColorSpace(name: CGColorSpace.sRGB))
        let source = UIColor(
            red: CGFloat(pixel[0]) / 255, green: CGFloat(pixel[1]) / 255, blue: CGFloat(pixel[2]) / 255, alpha: 1)
        return Color(uiColor: clamped(source))
    }

    private static func clamped(_ color: UIColor) -> UIColor {
        var hue: CGFloat = 0
        var saturation: CGFloat = 0
        var brightness: CGFloat = 0
        color.getHue(&hue, saturation: &saturation, brightness: &brightness, alpha: nil)
        let lightness = brightness * (1 - saturation / 2)
        let chroma = lightness == 0 || lightness == 1 ? 0 : (brightness - lightness) / min(lightness, 1 - lightness)
        let newLightness = min(lightness, maxLightness)
        let newSaturation = min(chroma, maxSaturation)
        let newBrightness = newLightness + newSaturation * min(newLightness, 1 - newLightness)
        let hsbSaturation = newBrightness == 0 ? 0 : 2 * (1 - newLightness / newBrightness)
        return UIColor(hue: hue, saturation: hsbSaturation, brightness: newBrightness, alpha: 1)
    }

    static func resolve(cabalID: String, pictureURL: URL?) async -> Color {
        let scene = color(forScene: .forSeed(cabalID))
        guard let pictureURL else { return scene }
        return await PhotoColorCache.shared.color(for: pictureURL) ?? scene
    }
}

private actor PhotoColorCache {
    static let shared = PhotoColorCache()
    private var cache: [URL: Color] = [:]

    func color(for url: URL) async -> Color? {
        if let hit = cache[url] { return hit }
        guard let image = await MonacoRemoteImageStore.avatars.image(for: url) else { return nil }
        let result = await Task.detached { SceneColor.color(forPhoto: image) }.value
        cache[url] = result
        return result
    }
}
