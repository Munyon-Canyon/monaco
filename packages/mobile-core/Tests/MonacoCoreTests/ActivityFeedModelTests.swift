import XCTest

@testable import MonacoCore

@MainActor
final class ActivityFeedModelTests: XCTestCase {
    private struct Fixed: ActivityFeedSource {
        let outcome: ActivityFeedOutcome

        func load() async -> ActivityFeedOutcome { outcome }
    }

    private func model(_ outcome: ActivityFeedOutcome) -> ActivityFeedModel {
        ActivityFeedModel(source: Fixed(outcome: outcome))
    }

    func testLoadingShowsTheLoadingPhaseUntilTheSourceAnswers() {
        XCTAssertEqual(model(.items([])).phase, .loading)
    }

    func testUnreadItemsLoadAsLoadedWithANewLabel() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: false)))

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.items.count, 4)
        XCTAssertTrue(model.hasUnread)
        XCTAssertEqual(model.groupLabel, "New")
    }

    func testReadItemsLabelTheGroupEarlier() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: true)))

        await model.load()

        XCTAssertFalse(model.hasUnread)
        XCTAssertEqual(model.groupLabel, "Earlier")
    }

    func testMarkAllReadClearsEveryUnreadItemAndSwitchesTheLabel() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: false)))
        await model.load()

        model.markAllRead()

        XCTAssertTrue(model.items.allSatisfy(\.isRead))
        XCTAssertFalse(model.hasUnread)
        XCTAssertEqual(model.groupLabel, "Earlier")
    }

    func testMarkAllReadWithNothingUnreadChangesNothing() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: true)))
        await model.load()
        let before = model.items

        model.markAllRead()

        XCTAssertEqual(model.items, before)
    }

    func testMarkReadClearsOnlyThatItem() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: false)))
        await model.load()

        model.markRead(id: "activity-reply")

        XCTAssertEqual(model.items.filter(\.isRead).map(\.id), ["activity-reply", "activity-deposit"])
        XCTAssertTrue(model.hasUnread)
    }

    func testMarkReadForAnUnknownIDChangesNothing() async {
        let model = model(.items(ActivityFeedItem.fixtures(isRead: false)))
        await model.load()
        let before = model.items

        model.markRead(id: "missing")

        XCTAssertEqual(model.items, before)
    }

    func testNoItemsIsTheEmptyPhase() async {
        let model = model(.items([]))

        await model.load()

        XCTAssertEqual(model.phase, .empty)
        XCTAssertFalse(model.hasUnread)
    }

    func testAFailedFirstLoadIsTheFailedPhase() async {
        let model = model(.failed)

        await model.load()

        XCTAssertEqual(model.phase, .failed)
    }

    func testTheLiveSourceIsComingSoonAndHoldsNoItems() async {
        let model = ActivityFeedModel()

        await model.load()

        XCTAssertEqual(model.phase, .comingSoon)
        XCTAssertTrue(model.items.isEmpty)
    }

    func testEveryItemDestinationIsAnExistingPushRoute() {
        let routes = ActivityFeedItem.fixtures(isRead: false).map(\.destination)

        XCTAssertEqual(
            routes[0], .proposal(proposalID: "activity-proposal-1", cabalID: "activity-cabal-1"))
        XCTAssertEqual(routes[3], .home)
    }

    func testAnUnreadItemReadsAsUnreadWithItsBodyAndMeta() {
        let item = ActivityFeedItem.fixtures(isRead: false)[1]

        XCTAssertEqual(
            item.spokenText,
            "Unread, The Microsoft proposal passed, The proposal reached its vote threshold., Big Ballers · 3h, Execution pending"
        )
    }

    func testAReadAccountUpdateReadsItsTitleAndMeta() {
        let item = ActivityFeedItem.fixtures(isRead: false)[3]

        XCTAssertEqual(item.spokenText, "Deposit confirmed, $100.00 · Yesterday")
    }

    func testThePreviewScenariosDriveEveryPhase() async {
        let expected: [ActivityFeedModel.PreviewScenario: ActivityFeedModel.Phase] = [
            .unread: .loaded, .read: .loaded, .empty: .empty, .error: .failed, .live: .comingSoon,
        ]
        for (scenario, phase) in expected {
            let model = ActivityFeedModel.preview(scenario)

            await model.load()

            XCTAssertEqual(model.phase, phase, "\(scenario)")
        }
    }

    func testThePreviewLoadingScenarioStaysLoading() async {
        let model = ActivityFeedModel.preview(.loading)

        let read = Task { await model.load() }
        for _ in 0..<20 { await Task.yield() }

        XCTAssertEqual(model.phase, .loading)
        read.cancel()
        await read.value
    }
}
