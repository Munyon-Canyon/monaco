import XCTest

@testable import MonacoCore

final class BoardRankDiffTests: XCTestCase {
    private func keys(_ ids: [String]) -> [BoardRowKey] {
        ids.enumerated().map { BoardRowKey(id: $1, rank: $0 + 1) }
    }

    func testAMoveUpIsPositiveAndTheRowItPassedMovesDown() {
        let changes = rankChanges(old: keys(["a", "b", "c"]), new: keys(["a", "c", "b"]))

        XCTAssertEqual(changes, ["c": 1, "b": -1])
    }

    func testAnUnchangedBoardHasNoChanges() {
        XCTAssertEqual(rankChanges(old: keys(["a", "b"]), new: keys(["a", "b"])), [:])
    }

    func testARowOnOnlyOneBoardIsNotAMove() {
        let changes = rankChanges(old: keys(["a", "b"]), new: keys(["a", "x", "b"]))

        XCTAssertEqual(changes, ["b": -1])
    }

    func testAnEmptyBoardHasNoChanges() {
        XCTAssertEqual(rankChanges(old: [], new: keys(["a"])), [:])
        XCTAssertEqual(rankChanges(old: keys(["a"]), new: []), [:])
    }

    func testTiedRanksAreComparedByRank() {
        let old = [BoardRowKey(id: "a", rank: 1), BoardRowKey(id: "b", rank: 1)]
        let new = [BoardRowKey(id: "a", rank: 1), BoardRowKey(id: "b", rank: 2)]

        XCTAssertEqual(rankChanges(old: old, new: new), ["b": -1])
    }
}
