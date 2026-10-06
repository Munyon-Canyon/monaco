import XCTest

@testable import MonacoCore

final class HomeDashboardDTOTests: XCTestCase {
    func testMonacoISO8601JSONDecoder_nonISO8601Timestamp_throwsDataCorrupted() {
        let json = #"{ "ts": "1789675200" }"#

        XCTAssertThrowsError(
            try monacoISO8601JSONDecoder().decode([String: Date].self, from: Data(json.utf8))
        ) { error in
            guard case DecodingError.dataCorrupted = error else {
                return XCTFail("expected dataCorrupted, got \(error)")
            }
        }
    }

    func testMonacoISO8601JSONDecoder_decodesWholeSecondAndFractionalUTCInstants() throws {
        let json = #"{ "plain": "2026-09-17T22:00:00Z", "fraction": "2026-09-17T22:00:00.5Z" }"#

        let dates = try monacoISO8601JSONDecoder().decode([String: Date].self, from: Data(json.utf8))

        XCTAssertEqual(dates["plain"]?.timeIntervalSince1970, 1_789_682_400)
        XCTAssertEqual(dates["fraction"]?.timeIntervalSince1970, 1_789_682_400.5)
    }
}
