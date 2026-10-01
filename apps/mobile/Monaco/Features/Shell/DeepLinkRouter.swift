import Foundation

protocol DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)?
}

enum DeepLinkRouter {
    static let handlers: [any DeepLinkHandler.Type] = [
        DepositDeepLink.self,
        ReferralDeepLink.self,
    ]

    @MainActor
    static func handle(_ url: URL, navigator: AppNavigator, handlers: [any DeepLinkHandler.Type] = handlers) {
        for handler in handlers {
            if let (route, tab) = handler.route(for: url) {
                navigator.open(route, in: tab)
                return
            }
        }
    }
}
