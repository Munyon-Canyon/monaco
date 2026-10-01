import Foundation

enum DepositDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        nil
    }
}
