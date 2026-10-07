import Foundation
import MonacoCore

enum ReferralDeepLink: DeepLinkHandler {
    struct Session {
        var isSignedIn: Bool
        var currentTab: MainTab
    }

    static var session: () -> Session = {
        let environment = AppDelegate.environment
        return Session(
            isSignedIn: environment?.isSignedIn ?? false,
            currentTab: environment?.navigator.selectedTab ?? .home)
    }
    static var pending = PendingReferralStore(store: UserDefaults.standard)

    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        guard let code = ReferralLink.parse(url) else { return nil }
        let session = session()
        guard session.isSignedIn else {
            try? pending.save(code, source: .universalLink, at: .now)
            return nil
        }
        return (ReferralRoute(code: code), session.currentTab)
    }
}
