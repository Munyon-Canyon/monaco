import Foundation

enum ReferralDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        nil
    }
}
