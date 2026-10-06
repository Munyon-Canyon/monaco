import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class FeedPreviewTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testTheItemsHarnessNarrowsByChipAndSearch() async {
        let model = FeedModel.preview(.items, clock: TestClock())
        await model.load()
        XCTAssertEqual(model.items.map(\.id), samples.map(\.id))
        await model.select(.cabals)
        XCTAssertEqual(model.items.map(\.kind), ["cabal_created", "member_joined"])
        await model.select(.all)
        await model.select(.following)
        XCTAssertEqual(model.items.count, samples.count)
    }

    func testTheEmptyAndFailedHarnessesShowTheirStates() async {
        let empty = FeedModel.preview(.empty, clock: TestClock())
        await empty.load()
        XCTAssertEqual(empty.phase, .empty(query: nil))
        let failed = FeedModel.preview(.failed, clock: TestClock())
        await failed.load()
        guard case .failed = failed.phase else { return XCTFail("expected failure, got \(failed.phase)") }
    }

    func testTheFollowsNobodyHarnessAsksToFollowInTheFollowingScope() async {
        let model = FeedModel.preview(.followsNobody, clock: TestClock())
        await model.load()
        XCTAssertEqual(model.phase, .empty(query: nil))
        await model.select(.following)
        XCTAssertEqual(model.phase, .followsNobody)
    }
}
