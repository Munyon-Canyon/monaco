import Foundation
import XCTest

@testable import MonacoAPI

final class FlexibleISO8601Tests: XCTestCase {
    func testFractionalDigitsAnOffsetAndGarbage() throws {
        let noon = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-09-30T12:00:00Z"))
        let transcoder = FlexibleISO8601()
        for raw in [
            "2026-09-30T12:00:00.123456789Z", "2026-09-30T12:00:00.123456Z", "2026-09-30T12:00:00.123Z",
            "2026-09-30T08:00:00.123-04:00",
        ] {
            XCTAssertEqual(try transcoder.decode(raw).timeIntervalSince(noon), 0.123, accuracy: 0.0001, raw)
        }
        XCTAssertEqual(try transcoder.decode("2026-09-30T12:00:00Z"), noon)
        XCTAssertThrowsError(try transcoder.decode("yesterday"))
    }
}
