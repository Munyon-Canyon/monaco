import UIKit

@MainActor
enum PresentedSheets {
    static func dismissAll() async {
        let root = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first(where: \.isKeyWindow)?
            .rootViewController
        await dismiss(from: root)
    }

    static func dismiss(from root: UIViewController?) async {
        guard let root, root.presentedViewController != nil else { return }
        await withCheckedContinuation { continuation in
            root.dismiss(animated: true) { continuation.resume() }
        }
    }
}
