import Foundation
import XCTest

@testable import MonacoCore

final class DisplayNameFixtureTests: XCTestCase {
    func testSharedDisplayNameFixtures() throws {
        let url = try XCTUnwrap(Bundle.module.url(forResource: "display_names", withExtension: "json"))
        let cases = try JSONDecoder().decode([Fixture].self, from: Data(contentsOf: url))
        for item in cases {
            switch DisplayNameRules.normalize(item.input) {
            case .success(let value): XCTAssertEqual(value, item.normalized)
            case .failure(let error): XCTAssertEqual(reason(for: error), item.reason)
            }
        }
    }

    func testFixtureMatchesTheBackendCopy() throws {
        let url = try XCTUnwrap(Bundle.module.url(forResource: "display_names", withExtension: "json"))
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { root.deleteLastPathComponent() }
        let backend =
            root
            .appendingPathComponent("apps/backend/internal/modules/identity/domain/testdata")
            .appendingPathComponent("display_names.json")
        XCTAssertEqual(try Data(contentsOf: url), try Data(contentsOf: backend))
    }

    private func reason(for error: DisplayNameValidationError) -> String {
        switch error {
        case .required: "required"
        case .tooLong: "too_long"
        case .invalidCharacters: "invalid_characters"
        case .needsLetterOrNumber: "needs_letter_or_number"
        }
    }
}

private struct Fixture: Decodable {
    let input: String
    let normalized: String?
    let reason: String?
}
