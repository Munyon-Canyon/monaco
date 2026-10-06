import MonacoCore
import Observation

@MainActor
final class PushRouter {
    private let environment: AppEnvironment
    private var held: PushRoute?
    private var waiting: Task<Void, Never>?

    init(environment: AppEnvironment) {
        self.environment = environment
    }

    func handle(_ route: PushRoute) {
        held = route
        guard environment.viewer != nil else {
            waitForSession()
            return
        }
        openHeld()
    }

    private func waitForSession() {
        guard waiting == nil else { return }
        waiting = Task {
            while environment.viewer == nil {
                await withCheckedContinuation { continuation in
                    withObservationTracking {
                        _ = environment.viewer
                    } onChange: {
                        continuation.resume()
                    }
                }
            }
            waiting = nil
            openHeld()
        }
    }

    private func openHeld() {
        guard let route = held else { return }
        held = nil
        open(route)
    }

    private func open(_ route: PushRoute) {
        let navigator = environment.navigator
        switch route {
        case .userProfile(let userID):
            navigator.open(UserProfileRoute(userID: userID), in: .home)
        case .chat(let cabalID):
            navigator.open(CabalRoute(id: cabalID), in: .cabals)
            navigator.open(ChatRoute(cabalID: cabalID), in: .cabals)
        case .transaction(let txnID, let cabalID):
            navigator.open(CabalRoute(id: cabalID), in: .cabals)
            navigator.open(TransactionRoute(cabalID: cabalID, transactionID: txnID), in: .cabals)
        case .proposal(let proposalID, let cabalID):
            navigator.open(CabalRoute(id: cabalID), in: .cabals)
            navigator.open(ProposalRoute(proposalID: proposalID), in: .cabals)
        case .feedItem(let feedItemID):
            navigator.open(FeedItemRoute(itemID: feedItemID), in: .feed)
        case .cabal(let cabalID):
            navigator.open(CabalRoute(id: cabalID), in: .cabals)
        case .home:
            navigator.selectedTab = .home
            navigator.homePath = []
        }
    }
}
