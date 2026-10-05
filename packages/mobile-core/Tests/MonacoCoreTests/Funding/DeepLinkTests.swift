import Foundation
import MonacoCore
import XCTest

final class DeepLinkTests: XCTestCase {
    private let session = "01890A5D-AC96-774B-BCCE-B302099A8057"

    func testTheRedirectParsesToItsLowercasedSession() {
        XCTAssertEqual(
            parse("monaco://deposit/complete?session=\(session)"),
            .depositComplete(sessionID: session.lowercased()))
    }

    func testAWrongSchemeOrHostIsNil() {
        XCTAssertNil(parse("https://deposit/complete?session=\(session)"))
        XCTAssertNil(parse("monaco://referral/complete?session=\(session)"))
    }

    func testAMissingOrRepeatedSessionIsNil() {
        XCTAssertNil(parse("monaco://deposit/complete"))
        XCTAssertNil(parse("monaco://deposit/complete?other=\(session)"))
        XCTAssertNil(parse("monaco://deposit/complete?session=\(session)&session=\(session)"))
    }

    func testANonUUIDSessionIsNil() {
        XCTAssertNil(parse("monaco://deposit/complete?session=not-a-uuid"))
        XCTAssertNil(parse("monaco://deposit/complete?session="))
    }

    func testAnExtraOrMissingPathIsNil() {
        XCTAssertNil(parse("monaco://deposit/complete/again?session=\(session)"))
        XCTAssertNil(parse("monaco://deposit?session=\(session)"))
        XCTAssertNil(parse("monaco://deposit/?session=\(session)"))
    }

    private func parse(_ string: String) -> DeepLink? {
        URL(string: string).flatMap(DeepLink.parse)
    }
}
