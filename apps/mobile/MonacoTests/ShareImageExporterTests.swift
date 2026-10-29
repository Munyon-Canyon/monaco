import MonacoCore
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ShareImageExporterTests {
    private static func alpha(of image: UIImage, x: Int, y: Int) -> UInt8? {
        guard let cgImage = image.cgImage else { return nil }
        var pixel = [UInt8](repeating: 0, count: 4)
        let drawn = pixel.withUnsafeMutableBytes { buffer -> Bool in
            guard
                let context = CGContext(
                    data: buffer.baseAddress, width: 1, height: 1, bitsPerComponent: 8, bytesPerRow: 4,
                    space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
            else { return false }
            let origin = CGPoint(x: -x, y: -(cgImage.height - 1 - y))
            let size = CGSize(width: cgImage.width, height: cgImage.height)
            context.draw(cgImage, in: CGRect(origin: origin, size: size))
            return true
        }
        return drawn ? pixel[3] : nil
    }

    @Test(arguments: [
        (ShareFormat.post, 1080, 1350), (.square, 1080, 1080), (.story, 1080, 1920), (.sticker, 1080, 612),
    ])
    func eachFormatExportsAtItsPixelSize(format: ShareFormat, width: Int, height: Int) throws {
        let image = ShareImageExporter.render(ShareSampleData.spec(), format: format)

        #expect(image.scale == 1)
        #expect(image.cgImage?.width == width)
        #expect(image.cgImage?.height == height)
        #expect(ShareImageExporter.pixelSize(format) == CGSize(width: width, height: height))
    }

    @Test func thePostImageIsOpaqueEvenAtItsRoundedCorner() throws {
        let image = ShareImageExporter.render(ShareSampleData.spec(), format: .post)

        #expect(Self.alpha(of: image, x: 0, y: 0) == 255)
        #expect(Self.alpha(of: image, x: 540, y: 675) == 255)
    }

    @Test func theStickerIsA1080WidePNGWithAlpha() throws {
        let image = ShareImageExporter.render(
            ShareSampleData.spec(), format: .sticker, eyes: ShareScreenModel.stickerEyes)

        #expect(image.cgImage?.width == 1080)
        let info = try #require(image.cgImage?.alphaInfo)
        #expect(info != .none && info != .noneSkipFirst && info != .noneSkipLast)
        #expect(Self.alpha(of: image, x: 0, y: 0) == 0)
        #expect(Self.alpha(of: image, x: 540, y: 306) == 255)
        let data = try #require(image.pngData())
        #expect(Array(data.prefix(4)) == [0x89, 0x50, 0x4E, 0x47])
    }

    @Test func theStickersEyesChangeTheExportedImage() throws {
        let off = ShareImageExporter.render(
            ShareSampleData.spec(), format: .sticker, eyes: ShareScreenModel.stickerEyes)
        let on = ShareImageExporter.render(ShareSampleData.spec(), format: .sticker, eyes: ShareEyes())

        #expect(on.pngData() != off.pngData())
    }

    @Test func theEyesChangeTheExportedImage() throws {
        let on = ShareImageExporter.render(ShareSampleData.spec(), format: .post)
        let off = ShareImageExporter.render(ShareSampleData.spec(eyes: ShareEyes(pnlDollars: false)), format: .post)

        #expect(on.pngData() != off.pngData())
    }

    @Test func aMissingLogoStillExportsAtFullSize() throws {
        let spec = ShareSampleData.spec()

        #expect(spec.tickerLine.logoURL == nil)
        #expect(ShareImageExporter.render(spec, format: .square).cgImage?.width == 1080)
    }
}
