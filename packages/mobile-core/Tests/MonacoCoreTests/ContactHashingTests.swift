import XCTest

@testable import MonacoCore

final class ContactHashingTests: XCTestCase {
    private static let usDigest = "36a2cef4ff9bf7a1abd2a93359136b870393be4106e6c5d19b72ff564f9deca4"

    func testNationalUSNumberHashesToThePinnedDigest() {
        let hashes = ContactHashing.hashes(for: ["(415) 555-0123"], defaultRegion: "US")
        XCTAssertEqual(hashes, [Self.usDigest])
    }

    func testAPlus44NumberHashesItsE164Form() {
        let fromInternational = ContactHashing.hashes(for: ["+44 20 7946 0958"], defaultRegion: "US")
        let fromCanonical = ContactHashing.hashes(for: ["+442079460958"], defaultRegion: "GB")
        XCTAssertEqual(fromInternational, fromCanonical)
        XCTAssertEqual(fromInternational.count, 1)
        XCTAssertEqual(fromInternational[0].count, 64)
        XCTAssertNotEqual(fromInternational[0], Self.usDigest)
    }

    func testJunkIsDropped() {
        let hashes = ContactHashing.hashes(
            for: ["call me", "Ada Lovelace", "", "555", "+14155550123"], defaultRegion: "US")
        XCTAssertEqual(hashes, [Self.usDigest])
    }

    func testDuplicateNumbersCollapseBeforeTheSort() {
        let hashes = ContactHashing.hashes(
            for: ["+442079460958", "+1 415 555 0123", "4155550123", "+44 20 7946 0958"],
            defaultRegion: "US")
        XCTAssertEqual(hashes.count, 2)
        XCTAssertEqual(hashes, hashes.sorted())
        XCTAssertTrue(hashes.contains(Self.usDigest))
    }

    func testChunksSplitAt2000() {
        let hashes = (0..<2001).map { String(format: "%064d", $0) }
        let chunks = ContactHashing.chunks(hashes)
        XCTAssertEqual(chunks.count, 2)
        XCTAssertEqual(chunks[0].count, 2000)
        XCTAssertEqual(chunks[1], [hashes[2000]])
    }
}
