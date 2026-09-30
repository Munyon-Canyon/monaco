import MonacoAPI
import XCTest

final class HintTests: XCTestCase {
    func testEveryKeyFormParses() {
        XCTAssertEqual(HintKey(wire: "user:9f2c"), .user("9f2c"))
        XCTAssertEqual(HintKey(wire: "cabal:42"), .cabal("42"))
        XCTAssertEqual(HintKey(wire: "global"), .global)
        XCTAssertEqual(
            Hint(key: "cabal:42", what: "updated", id: "7"), .changed(.cabal("42"), what: "updated", id: "7"))
    }

    func testMalformedKeysAndTokensAreRejected() {
        for (key, what) in [
            ("cabal:", "updated"),
            ("user:", "updated"),
            ("cabal", "updated"),
            ("global:1", "updated"),
            ("group:42", "updated"),
            ("", "updated"),
            ("cabal:42", ""),
            ("cabal:42", "Updated"),
            ("cabal:42", "up-dated"),
            ("cabal:42", "up dated"),
        ] {
            XCTAssertNil(Hint(key: key, what: what, id: "1"), "\(key)/\(what)")
        }
    }

    func testDescriptionIsTheLogForm() {
        XCTAssertEqual(Hint.changed(.cabal("42"), what: "updated", id: "3").description, "cabal:42/updated")
        XCTAssertEqual(Hint.changed(.user("9f2c"), what: "ping_echoed", id: "3").description, "user:9f2c/ping_echoed")
        XCTAssertEqual(Hint.changed(.global, what: "feed", id: "3").description, "global/feed")
        XCTAssertEqual(Hint.resync.description, "resync")
    }
}
