import Foundation
import XCTest

@testable import MonacoCore

@MainActor
final class FrequentEmojiStoreTests: XCTestCase {
    private let suite = "FrequentEmojiStoreTests-\(UUID().uuidString)"

    override func tearDown() async throws {
        UserDefaults(suiteName: suite)?.removePersistentDomain(forName: suite)
    }

    private func store() throws -> FrequentEmojiStore {
        FrequentEmojiStore(defaults: try XCTUnwrap(UserDefaults(suiteName: suite)))
    }

    func testItStartsWithTheQuickSixAndTheFramesLastTwo() async throws {
        XCTAssertEqual(try store().emojis, ["👍", "❤️", "😂", "🎉", "👀", "💪", "🙌", "🤝"])
    }

    func testTheLatestPickGoesFirstAndMovesUpWithoutRepeating() async throws {
        let store = try store()
        store.record("🔥")
        store.record("🤝")
        XCTAssertEqual(store.emojis, ["🤝", "🔥", "👍", "❤️", "😂", "🎉", "👀", "💪"])
    }

    func testItKeepsOnlyTheLastEight() async throws {
        let store = try store()
        let picks = ["🔥", "🚀", "🌙", "🍕", "🐶", "⚡️", "🎸", "🧠", "🏆", "🫡"]
        picks.forEach(store.record)
        XCTAssertEqual(store.emojis, Array(picks.reversed().prefix(8)))
        XCTAssertEqual(store.emojis.count, FrequentEmojiStore.capacity)
    }

    func testAPickSurvivesANewStoreOnTheSameDefaults() async throws {
        try store().record("🔥")
        XCTAssertEqual(try store().emojis.first, "🔥")
    }
}
