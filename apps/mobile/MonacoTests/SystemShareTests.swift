import Foundation
import MonacoCore
import Testing
import UIKit
import UniformTypeIdentifiers

@testable import Monaco

@MainActor
struct SystemShareTests {
    private static let url = URL(string: "https://monacolabs.xyz/ANDRES?c=ac-capital") ?? SharePayload.fallbackURL

    private static func image() -> UIImage {
        UIGraphicsImageRenderer(size: CGSize(width: 8, height: 8)).image { context in
            UIColor.green.setFill()
            context.fill(CGRect(x: 0, y: 0, width: 8, height: 8))
        }
    }

    private static let controller = UIActivityViewController(activityItems: [], applicationActivities: nil)

    @Test func aPayloadWithAnImageSharesTheImageAsItsOwnItemBesideTheLink() throws {
        let image = Self.image()
        let items = SystemShare.activityItems(for: SharePayload(title: "AC Capital", url: Self.url, image: image))

        #expect(items.count == 2)
        let imageItem = try #require(items.first as? ShareImageItem)
        #expect((imageItem.activityViewController(Self.controller, itemForActivityType: nil) as? UIImage) === image)
        #expect((imageItem.activityViewControllerPlaceholderItem(Self.controller) as? UIImage) === image)
        #expect(
            imageItem.activityViewController(Self.controller, dataTypeIdentifierForActivityType: nil)
                == UTType.png.identifier)
        let linkItem = try #require(items.last as? ShareLinkItem)
        #expect(linkItem.activityViewController(Self.controller, itemForActivityType: nil) as? URL == Self.url)
    }

    @Test func aPayloadWithoutAnImageSharesTheLinkAlone() throws {
        let items = SystemShare.activityItems(for: SharePayload(title: "AC Capital", url: Self.url))

        #expect(items.count == 1)
        let linkItem = try #require(items.first as? ShareLinkItem)
        #expect(linkItem.activityViewController(Self.controller, itemForActivityType: nil) as? URL == Self.url)
    }
}
