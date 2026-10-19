#if DEBUG
import Foundation

extension ActivityFeedModel {
    public enum PreviewScenario: String, CaseIterable, Sendable {
        case unread
        case read
        case empty
        case loading
        case error
        case live
    }

    public static func preview(_ scenario: PreviewScenario) -> ActivityFeedModel {
        ActivityFeedModel(source: ActivityFeedPreviewSource(scenario: scenario))
    }
}

struct ActivityFeedPreviewSource: ActivityFeedSource {
    let scenario: ActivityFeedModel.PreviewScenario

    func load() async -> ActivityFeedOutcome {
        switch scenario {
        case .unread: .items(ActivityFeedItem.fixtures(isRead: false))
        case .read: .items(ActivityFeedItem.fixtures(isRead: true))
        case .empty: .items([])
        case .loading: await Self.never()
        case .error: .failed
        case .live: await LiveActivityFeedSource().load()
        }
    }

    private static func never() async -> ActivityFeedOutcome {
        try? await Task.sleep(for: .seconds(3_600))
        return .failed
    }
}

extension ActivityFeedItem {
    static func fixtures(isRead: Bool) -> [ActivityFeedItem] {
        typealias Copy = ActivityFeedCopy.Fixture
        return [
            ActivityFeedItem(
                id: "activity-reply", kind: .reply, title: Copy.replyTitle, body: Copy.replyBody,
                cabalName: Copy.replyCabal, age: Copy.replyAge, actorName: "Mia Chen", actorID: "activity-actor-1",
                destination: .proposal(proposalID: "activity-proposal-1", cabalID: "activity-cabal-1"),
                isRead: isRead),
            ActivityFeedItem(
                id: "activity-passed", kind: .proposalPassed, title: Copy.passedTitle, body: Copy.passedBody,
                cabalName: Copy.passedCabal, age: Copy.passedAge, actorName: "Ben Ortiz", actorID: "activity-actor-2",
                badge: ActivityFeedBadge(Copy.passedBadge, tone: .warn),
                destination: .proposal(proposalID: "activity-proposal-2", cabalID: "activity-cabal-1"),
                isRead: isRead),
            ActivityFeedItem(
                id: "activity-vote", kind: .vote, title: Copy.voteTitle, body: Copy.voteBody,
                cabalName: Copy.voteCabal, age: Copy.voteAge, actorName: "Sam Patel", actorID: "activity-actor-3",
                destination: .proposal(proposalID: "activity-proposal-3", cabalID: "activity-cabal-2"),
                isRead: isRead),
            ActivityFeedItem(
                id: "activity-deposit", kind: .accountUpdate, title: Copy.depositTitle, body: Copy.depositAmount,
                age: Copy.depositAge, destination: .home, isRead: true),
        ]
    }
}
#endif
