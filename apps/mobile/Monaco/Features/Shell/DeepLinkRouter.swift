import Foundation
import MonacoCore
import MonacoReferrals

enum DeepLinkAction {
    case open(any AppRoute, in: MainTab)
    case voteHold(proposalID: String)
    case signedOutLanding(InviteLandingRoute)
    case handled
}

protocol DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction?
}

enum DeepLinkRouter {
    static let handlers: [any DeepLinkHandler.Type] = [ReferralDeepLink.self]

    @MainActor
    static func handle(
        _ url: URL,
        navigator: AppNavigator,
        handlers: [any DeepLinkHandler.Type] = handlers,
        sessionRestored: @MainActor () async -> Void = {},
        dismissPresented: @MainActor () async -> Void = PresentedSheets.dismissAll
    ) async {
        await sessionRestored()
        for handler in handlers {
            if let action = handler.action(for: url) {
                if case .handled = action { return }
                await dismissPresented()
                perform(action, navigator: navigator)
                return
            }
        }
    }

    @MainActor
    static func resumePending(
        navigator: AppNavigator,
        store: PendingLinkTargetStore = PendingLinkTargetStore(store: UserDefaults.standard),
        now: () -> Date = { .now },
        dismissPresented: @MainActor () async -> Void = PresentedSheets.dismissAll
    ) async {
        guard let target = store.take(now: now()) else { return }
        switch target.kind {
        case .join:
            guard let code = ReferralCode(target.code) else { return }
            await dismissPresented()
            navigator.open(InviteLandingRoute(code: code, cabalID: target.cabalID), in: .cabals)
        case .vote:
            guard let proposalID = target.proposalID else { return }
            await dismissPresented()
            navigator.presentVoteHold(proposalID: proposalID, pick: nil)
        }
    }

    @MainActor
    private static func perform(_ action: DeepLinkAction, navigator: AppNavigator) {
        switch action {
        case .open(let route, let tab): navigator.open(route, in: tab)
        case .voteHold(let proposalID): navigator.presentVoteHold(proposalID: proposalID, pick: nil)
        case .signedOutLanding(let route): navigator.presentSignedOutLanding(route)
        case .handled: break
        }
    }
}
