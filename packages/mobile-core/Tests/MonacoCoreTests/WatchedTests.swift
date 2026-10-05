import MonacoTestClock
import XCTest

final class WatchedTests: XCTestCase {
    private let instantly: @Sendable (Duration) async -> Void = { _ in }

    func testUntilWithinReturnsTrueWhenThePredicateAlreadyHolds() async {
        let watched = Watched(1)

        let reached = await watched.until(within: .seconds(1), sleep: instantly) { $0 == 1 }

        XCTAssertTrue(reached)
    }

    func testUntilWithinReturnsFalseWhenTheLimitPassesFirst() async {
        let watched = Watched(1)

        let reached = await watched.until(within: .seconds(1), sleep: instantly) { $0 == 2 }

        XCTAssertFalse(reached)
    }

    func testUntilWithinReturnsTrueWhenAMutationArrivesBeforeTheLimit() async {
        let watched = Watched(1)
        let (gate, release) = AsyncStream.makeStream(of: Void.self)

        let waiting = Task {
            await watched.until(within: .seconds(1), sleep: { _ in for await _ in gate {} }) { $0 == 2 }
        }
        watched.mutate { $0 = 2 }
        let reached = await waiting.value
        release.finish()

        XCTAssertTrue(reached)
    }
}
