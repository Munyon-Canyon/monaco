import Foundation
import MonacoCore

enum DepositDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        guard case .depositComplete(let sessionID) = DeepLink.parse(url) else { return nil }
        return (DepositCompleteRoute(sessionID: sessionID), .home)
    }
}
