import Foundation
import MonacoCore
import XCTest

@MainActor
final class WatchlistModelTests: XCTestCase {
    func testStarsAreKeptInTheOrderTheyWereAdded() async {
        let model = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore())

        XCTAssertEqual(model.toggle("NVDAx"), .added)
        XCTAssertEqual(model.toggle("AAPLx"), .added)
        XCTAssertEqual(model.toggle("MSFTx"), .added)

        XCTAssertEqual(model.symbols, ["NVDAx", "AAPLx", "MSFTx"])
        XCTAssertTrue(model.isWatched("AAPLx"))
        XCTAssertFalse(model.isWatched("TSLAx"))
    }

    func testTogglingAWatchedSymbolRemovesItAndKeepsTheRestInOrder() async {
        let model = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore())
        for symbol in ["NVDAx", "AAPLx", "MSFTx"] { model.toggle(symbol) }

        XCTAssertEqual(model.toggle("AAPLx"), .removed)

        XCTAssertEqual(model.symbols, ["NVDAx", "MSFTx"])
        XCTAssertFalse(model.isWatched("AAPLx"))
    }

    func testRemoveForgetsASymbolAndIgnoresOneThatIsNotWatched() async {
        let store = InMemoryWatchlistStore(["u1": ["NVDAx", "AAPLx"]])
        let model = WatchlistModel(userID: "u1", store: store)

        model.remove("AAPLx")
        model.remove("TSLAx")

        XCTAssertEqual(model.symbols, ["NVDAx"])
        XCTAssertEqual(store.symbols(for: "u1"), ["NVDAx"])
    }

    func testTheFiftyFirstStarIsRefusedAndNothingIsStored() async {
        let store = InMemoryWatchlistStore()
        let model = WatchlistModel(userID: "u1", store: store)
        for index in 0..<WatchlistModel.capacity { XCTAssertEqual(model.toggle("S\(index)"), .added) }

        XCTAssertEqual(model.toggle("ONEMORE"), .full)

        XCTAssertEqual(model.symbols.count, 50)
        XCTAssertFalse(model.isWatched("ONEMORE"))
        XCTAssertEqual(store.symbols(for: "u1").count, 50)
        XCTAssertEqual(WatchlistCopy.fullMessage, "Your watchlist holds up to 50 stocks.")
    }

    func testAFullWatchlistStillLetsAStarComeOff() async {
        let model = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore())
        for index in 0..<WatchlistModel.capacity { model.toggle("S\(index)") }

        XCTAssertEqual(model.toggle("S3"), .removed)
        XCTAssertEqual(model.toggle("ONEMORE"), .added)
    }

    func testEveryChangeIsWrittenThroughTheStore() async {
        let store = InMemoryWatchlistStore()
        let model = WatchlistModel(userID: "u1", store: store)

        model.toggle("NVDAx")
        model.toggle("AAPLx")

        XCTAssertEqual(store.symbols(for: "u1"), ["NVDAx", "AAPLx"])
        XCTAssertEqual(WatchlistModel(userID: "u1", store: store).symbols, ["NVDAx", "AAPLx"])
    }

    func testAnotherUserOnTheSameDeviceNeverSeesTheFirstUsersList() async {
        let store = InMemoryWatchlistStore()
        let first = WatchlistModel(userID: "u1", store: store)
        first.toggle("NVDAx")

        let second = WatchlistModel(userID: "u2", store: store)

        XCTAssertTrue(second.symbols.isEmpty)
        second.toggle("AAPLx")
        XCTAssertEqual(first.symbols, ["NVDAx"])
        XCTAssertEqual(store.symbols(for: "u1"), ["NVDAx"])
        XCTAssertEqual(store.symbols(for: "u2"), ["AAPLx"])
    }

    func testAStoredListWithRepeatsBlanksOrTooManySymbolsIsCleanedOnLoad() async {
        let crowded = (0..<60).map { "S\($0)" }
        let store = InMemoryWatchlistStore(["u1": ["NVDAx", "", "NVDAx", "AAPLx"], "u2": crowded])

        XCTAssertEqual(WatchlistModel(userID: "u1", store: store).symbols, ["NVDAx", "AAPLx"])
        XCTAssertEqual(WatchlistModel(userID: "u2", store: store).symbols, Array(crowded.prefix(50)))
    }

    func testTheCountLineReadsTheWayTheScreenDoes() async {
        XCTAssertEqual(WatchlistCopy.countLine(0), "No stocks selected")
        XCTAssertEqual(WatchlistCopy.countLine(1), "1 stock selected")
        XCTAssertEqual(WatchlistCopy.countLine(3), "3 stocks selected")
    }

    func testEachUsersListLivesUnderItsOwnKeyAsAJSONArray() async throws {
        XCTAssertEqual(WatchlistRecord.key(for: "u1"), "markets-watchlist.u1")
        XCTAssertNotEqual(WatchlistRecord.key(for: "u1"), WatchlistRecord.key(for: "u2"))
        let data = WatchlistRecord.encode(["NVDAx", "AAPLx"])
        XCTAssertEqual(String(decoding: data, as: UTF8.self), #"["NVDAx","AAPLx"]"#)
        XCTAssertEqual(WatchlistRecord.decode(data), ["NVDAx", "AAPLx"])
    }

    func testAStoredValueThatIsMissingOrNotAListOfSymbolsReadsAsEmpty() async {
        XCTAssertEqual(WatchlistRecord.decode(nil), [])
        XCTAssertEqual(WatchlistRecord.decode(Data("not json".utf8)), [])
        XCTAssertEqual(WatchlistRecord.decode(Data(#"{"symbols":["NVDAx"]}"#.utf8)), [])
        XCTAssertEqual(WatchlistRecord.decode(Data("[1,2]".utf8)), [])
    }
}
