import UIKit

@MainActor
enum TestWindow {
    static func close(_ window: UIWindow) {
        window.isHidden = true
        window.rootViewController = nil
    }
}
