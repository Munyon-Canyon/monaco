import Foundation
import MonacoCore
import MonacoReferrals

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

    static var targets = PendingLinkTargetStore(store: UserDefaults.standard)

    static func action(for url: URL) -> DeepLinkAction? {
        guard let target = ReferralLink.parseTarget(url) else { return nil }
        let session = session()
        guard session.isSignedIn else {
            try? pending.save(target.code, source: .universalLink, at: .now)
            guard let cabalID = target.cabalID else { return .handled }
            targets.save(target, now: .now)
            return .signedOutLanding(InviteLandingRoute(code: target.code, cabalID: cabalID))
        }
        if let proposalID = target.proposalID { return .voteHold(proposalID: proposalID) }
        if let cabalID = target.cabalID {
            return .open(InviteLandingRoute(code: target.code, cabalID: cabalID), in: .cabals)
        }
        return .open(ReferralRoute(code: target.code), in: session.currentTab)
    }
}
