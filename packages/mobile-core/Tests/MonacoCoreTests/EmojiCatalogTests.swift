import Foundation
import XCTest

@testable import MonacoCore

final class EmojiCatalogTests: XCTestCase {
    private static let url = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("apps/mobile/Monaco/Features/Reactions/emoji.json")

    private func catalog() throws -> EmojiCatalog {
        try EmojiCatalog.load(from: Data(contentsOf: Self.url))
    }

    func testPartyFindsThePartyPopper() throws {
        XCTAssertTrue(try catalog().search("party").map(\.emoji).contains("🎉"))
    }

    func testThumbsFindsThumbsUp() throws {
        XCTAssertTrue(try catalog().search("thumbs").map(\.emoji).contains("👍"))
    }

    func testEyesPutsTheEyesFirst() throws {
        XCTAssertEqual(try catalog().search("eyes").first?.emoji, "👀")
    }

    func testSearchIgnoresCaseAndDiacritics() throws {
        let catalog = try catalog()
        XCTAssertEqual(catalog.search("PÀRTY").map(\.emoji), catalog.search("party").map(\.emoji))
        XCTAssertEqual(catalog.search("pinata").first?.emoji, "🪅")
    }

    func testEveryWordOfTheQueryMustMatchAPrefix() throws {
        let found = try catalog().search("thumbs up").map(\.emoji)
        XCTAssertTrue(found.contains("👍"))
        XCTAssertFalse(found.contains("🎉"))
    }

    func testAnEmptyQueryFindsNothing() throws {
        XCTAssertTrue(try catalog().search("  ").isEmpty)
    }

    func testNameForReadsTheCLDRName() throws {
        let catalog = try catalog()
        XCTAssertEqual(catalog.name(for: "👀"), "eyes")
        XCTAssertEqual(catalog.name(for: "🎉"), "party popper")
        XCTAssertEqual(catalog.name(for: "❤"), "red heart")
        XCTAssertNil(catalog.name(for: "not an emoji"))
    }

    func testTheFileIsUnderThreeHundredKilobytes() throws {
        let size = try Data(contentsOf: Self.url).count
        XCTAssertLessThan(size, 300 * 1024)
    }

    func testNoSkinToneVariantAppears() throws {
        let tones = 0x1F3FB...0x1F3FF
        for entry in try catalog().entries {
            XCTAssertFalse(entry.emoji.unicodeScalars.contains { tones.contains(Int($0.value)) }, entry.name)
        }
    }

    func testEveryGroupHasEmoji() throws {
        let catalog = try catalog()
        for group in EmojiGroup.allCases {
            XCTAssertFalse(catalog.entries(in: group).isEmpty, group.rawValue)
        }
    }
}
