import MonacoAPI
import MonacoCore
import XCTest

final class FeedAccentTests: XCTestCase {
    func testNeutralItemsTakeTheirKindsAccent() {
        XCTAssertEqual(FeedAccent(kind: "proposal", tone: "neutral"), .ink)
        XCTAssertEqual(FeedAccent(kind: "trade", tone: "neutral"), .ink)
        XCTAssertEqual(FeedAccent(kind: "price_move", tone: "neutral"), .muted)
        XCTAssertEqual(FeedAccent(kind: "cabal_created", tone: "neutral"), .muted)
        XCTAssertEqual(FeedAccent(kind: "member_joined", tone: "neutral"), .muted)
        XCTAssertEqual(FeedAccent(kind: "something_new", tone: "neutral"), .muted)
    }

    func testToneOverridesTheKind() {
        for kind in FeedKind.allCases.map(\.rawValue) {
            XCTAssertEqual(FeedAccent(kind: kind, tone: "positive"), .positive, kind)
            XCTAssertEqual(FeedAccent(kind: kind, tone: "negative"), .negative, kind)
        }
    }
}
