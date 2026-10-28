import LinkPresentation
import MonacoCore
import SwiftUI
import UIKit
import UniformTypeIdentifiers

struct SharePayload {
    let title: String
    let url: URL
    var image: UIImage?

    static func card(_ content: ShareCardContent, image: UIImage?) -> SharePayload {
        SharePayload(title: content.title, url: content.shareURL, image: image)
    }
}

final class ShareLinkItem: NSObject, UIActivityItemSource {
    private let payload: SharePayload

    init(_ payload: SharePayload) {
        self.payload = payload
    }

    func activityViewControllerPlaceholderItem(_ controller: UIActivityViewController) -> Any {
        payload.url
    }

    func activityViewController(
        _ controller: UIActivityViewController, itemForActivityType activityType: UIActivity.ActivityType?
    ) -> Any? {
        payload.url
    }

    func activityViewController(
        _ controller: UIActivityViewController, subjectForActivityType activityType: UIActivity.ActivityType?
    ) -> String {
        payload.title
    }

    func activityViewControllerLinkMetadata(_ controller: UIActivityViewController) -> LPLinkMetadata? {
        let metadata = LPLinkMetadata()
        metadata.originalURL = payload.url
        metadata.url = payload.url
        metadata.title = payload.title
        if let image = payload.image {
            let provider = NSItemProvider(object: image)
            metadata.imageProvider = provider
            metadata.iconProvider = provider
        }
        return metadata
    }
}

final class ShareImageItem: NSObject, UIActivityItemSource {
    private let image: UIImage

    init(_ image: UIImage) {
        self.image = image
    }

    func activityViewControllerPlaceholderItem(_ controller: UIActivityViewController) -> Any {
        image
    }

    func activityViewController(
        _ controller: UIActivityViewController, itemForActivityType activityType: UIActivity.ActivityType?
    ) -> Any? {
        image
    }

    func activityViewController(
        _ controller: UIActivityViewController, dataTypeIdentifierForActivityType activityType: UIActivity.ActivityType?
    ) -> String {
        UTType.png.identifier
    }
}

@MainActor
enum SystemShare {
    static func activityItems(for payload: SharePayload) -> [Any] {
        guard let image = payload.image else { return [ShareLinkItem(payload)] }
        return [ShareImageItem(image), ShareLinkItem(payload)]
    }

    static func present(_ payload: SharePayload) {
        guard let top = topController() else { return }
        let items = activityItems(for: payload)
        let controller = UIActivityViewController(activityItems: items, applicationActivities: nil)
        controller.excludedActivityTypes = [.assignToContact, .print]
        controller.sheetPresentationController?.detents = [.medium()]
        if let popover = controller.popoverPresentationController {
            popover.sourceView = top.view
            popover.sourceRect = CGRect(x: top.view.bounds.midX, y: top.view.bounds.midY, width: 0, height: 0)
            popover.permittedArrowDirections = []
        }
        top.present(controller, animated: true)
    }

    private static func topController() -> UIViewController? {
        let scene = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .first { $0.activationState == .foregroundActive }
        var top = scene?.keyWindow?.rootViewController
        while let next = top?.presentedViewController { top = next }
        return top
    }
}
