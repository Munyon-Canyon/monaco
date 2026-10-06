import XCTest

@testable import MonacoCore

final class GroupsTabDTOTests: XCTestCase {

    // MARK: - Fixture decoding

    func testGroupSearchResponseDTO_decodesFixturePayload() throws {
        // Arrange
        let fixtureURL = try XCTUnwrap(
            Bundle.module.url(forResource: "groups_search", withExtension: "json")
        )
        let data = try Data(contentsOf: fixtureURL)

        // Act
        let dto = try monacoISO8601JSONDecoder().decode(GroupSearchResponseDTO.self, from: data)

        // Assert
        XCTAssertEqual(dto.groups.count, 2)
        let joined = dto.groups[0]
        XCTAssertEqual(joined.groupID, "3f9a1b2c-4d5e-4f6a-8b7c-9d0e1f2a3b4c")
        XCTAssertEqual(joined.name, "Weekend investors")
        XCTAssertEqual(joined.memberCount, 8)
        XCTAssertEqual(joined.potValueUsd, "548.20")
        XCTAssertEqual(joined.percentReturn, "0.124")
        XCTAssertEqual(joined.dollarPnl, "+48.20")
        XCTAssertTrue(joined.isJoined)
        XCTAssertEqual(joined.joinMode, .open)

        let requestMode = dto.groups[1]
        XCTAssertEqual(requestMode.name, "Rent money")
        XCTAssertNil(requestMode.percentReturn)
        XCTAssertFalse(requestMode.isJoined)
        XCTAssertEqual(requestMode.joinMode, .request)

        XCTAssertEqual(dto.nextCursor, "eyJvZmZzZXQiOjIwfQ==")
    }

    func testGroupJoinMode_unknownRawValue_decodesAsRequest() throws {
        // Arrange
        let json = #"{"joinMode":"password"}"#
        struct Wrapper: Decodable { let joinMode: GroupJoinMode }

        // Act
        let wrapper = try JSONDecoder().decode(Wrapper.self, from: Data(json.utf8))

        // Assert
        XCTAssertEqual(wrapper.joinMode, .request)
    }

    func testGroupJoinMode_knownRawValues_decodeAsExpected() throws {
        struct Wrapper: Decodable { let joinMode: GroupJoinMode }

        let open = try JSONDecoder().decode(Wrapper.self, from: Data(#"{"joinMode":"open"}"#.utf8))
        let request = try JSONDecoder().decode(Wrapper.self, from: Data(#"{"joinMode":"request"}"#.utf8))

        XCTAssertEqual(open.joinMode, .open)
        XCTAssertEqual(request.joinMode, .request)
    }

    // MARK: - GroupSearchResponseDTO.nextCursor

    func testGroupSearchResponseDTO_nullNextCursor_decodesAsNil() throws {
        // Arrange
        let json = #"{"groups":[],"nextCursor":null}"#

        // Act
        let dto = try monacoISO8601JSONDecoder().decode(GroupSearchResponseDTO.self, from: Data(json.utf8))

        // Assert
        XCTAssertNil(dto.nextCursor)
        XCTAssertTrue(dto.groups.isEmpty)
    }

    // MARK: - GroupDiscoveryDestination

    func testGroupDiscoveryDestination_isJoined_alwaysDetail() {
        // Arrange / Act / Assert
        XCTAssertEqual(GroupDiscoveryDestination(isJoined: true, joinMode: .open), .detail)
        XCTAssertEqual(GroupDiscoveryDestination(isJoined: true, joinMode: .request), .detail)
    }

    func testGroupDiscoveryDestination_notJoined_openMode_isJoin() {
        // Arrange / Act / Assert
        XCTAssertEqual(GroupDiscoveryDestination(isJoined: false, joinMode: .open), .join)
    }

    func testGroupDiscoveryDestination_notJoined_requestMode_isRequestToJoin() {
        // Arrange / Act / Assert
        XCTAssertEqual(GroupDiscoveryDestination(isJoined: false, joinMode: .request), .requestToJoin)
    }

    // MARK: - GroupPnLChartModel.drawable

    // MARK: - GroupSearchQuery.normalized

    func testGroupSearchQuery_normalized_emptyString_isNil() {
        // Arrange / Act / Assert
        XCTAssertNil(GroupSearchQuery.normalized(""))
    }

    func testGroupSearchQuery_normalized_singleTrimmedCharacter_isNil() {
        // Arrange / Act / Assert
        XCTAssertNil(GroupSearchQuery.normalized(" a "))
    }

    func testGroupSearchQuery_normalized_twoCharacters_isAccepted() {
        // Arrange / Act / Assert
        XCTAssertEqual(GroupSearchQuery.normalized("ab"), "ab")
    }

    func testGroupSearchQuery_normalized_trimsWhitespace() {
        // Arrange / Act / Assert
        XCTAssertEqual(GroupSearchQuery.normalized("  weekend  "), "weekend")
    }

    func testGroupSearchQuery_normalized_cappedAt64Characters() {
        // Arrange
        let raw = String(repeating: "x", count: 100)

        // Act
        let normalized = GroupSearchQuery.normalized(raw)

        // Assert
        XCTAssertEqual(normalized?.count, 64)
        XCTAssertEqual(normalized, String(repeating: "x", count: 64))
    }

    func testSignedUsdFormatter_formatsGainsLossesAndZero() {
        XCTAssertEqual(SignedUsdFormatter.format("+48.20"), "+$48.20")
        XCTAssertEqual(SignedUsdFormatter.format("-3.10"), "\u{2212}$3.10")
        XCTAssertEqual(SignedUsdFormatter.format("+1234.5"), "+$1,234.50")
        XCTAssertEqual(SignedUsdFormatter.format("-0.00"), "$0.00")
    }

    func testSignedUsdFormatter_isLossOnlyBelowZero() {
        XCTAssertTrue(SignedUsdFormatter.isLoss("-3.10"))
        XCTAssertFalse(SignedUsdFormatter.isLoss("+3.10"))
        XCTAssertFalse(SignedUsdFormatter.isLoss("-0.00"))
        XCTAssertFalse(SignedUsdFormatter.isLoss("garbage"))
    }

    // MARK: - Negative zero, dust and the typographic minus (plan §8)

    func testSignedUsdFormatter_zeroAndDustRenderUnsignedZero() {
        XCTAssertEqual(SignedUsdFormatter.format("+0.00"), "$0.00")
        XCTAssertEqual(SignedUsdFormatter.format("-0.00"), "$0.00")
        XCTAssertEqual(SignedUsdFormatter.format("-0.001"), "$0.00")
        XCTAssertEqual(SignedUsdFormatter.format("0.004"), "$0.00")
        XCTAssertEqual(SignedUsdFormatter.format("0"), "$0.00")
    }

    func testSignedUsdFormatter_signsUseTypographicMinusAndPadCents() {
        XCTAssertEqual(SignedUsdFormatter.format("-7.6"), "\u{2212}$7.60")
        XCTAssertEqual(SignedUsdFormatter.format("+48.2"), "+$48.20")
        XCTAssertEqual(SignedUsdFormatter.format("48.2"), "+$48.20")
        XCTAssertEqual(SignedUsdFormatter.format("-0.005"), "\u{2212}$0.01")
        XCTAssertEqual(SignedUsdFormatter.format("\u{2212}7.60"), "\u{2212}$7.60")
        XCTAssertEqual(SignedUsdFormatter.format(" -1234567.891 "), "\u{2212}$1,234,567.89")
        XCTAssertFalse(SignedUsdFormatter.format("-7.6").contains("-"), "ASCII hyphen must never reach the screen")
    }

    func testSignedUsdFormatter_garbageRendersDash() {
        XCTAssertEqual(SignedUsdFormatter.format("garbage"), "—")
        XCTAssertEqual(SignedUsdFormatter.format(""), "—")
        XCTAssertEqual(SignedUsdFormatter.format("-"), "—")
        XCTAssertEqual(SignedUsdFormatter.format("1e5"), "—")
    }

    func testSignedUsdFormatter_isLossIgnoresDustAndAcceptsBothMinusSigns() {
        XCTAssertFalse(SignedUsdFormatter.isLoss("-0.001"))
        XCTAssertTrue(SignedUsdFormatter.isLoss("-0.006"))
        XCTAssertTrue(SignedUsdFormatter.isLoss("\u{2212}7.60"))
        XCTAssertFalse(SignedUsdFormatter.isLoss("+48.2"))
    }

    func testSignedUsdFormatter_isZero() {
        XCTAssertTrue(SignedUsdFormatter.isZero("+0.00"))
        XCTAssertTrue(SignedUsdFormatter.isZero("-0.00"))
        XCTAssertTrue(SignedUsdFormatter.isZero("-0.001"))
        XCTAssertFalse(SignedUsdFormatter.isZero("-7.6"))
        XCTAssertFalse(SignedUsdFormatter.isZero("+48.2"))
        XCTAssertFalse(SignedUsdFormatter.isZero("garbage"))
    }

    func testSignedUsdFormatter_parse() {
        XCTAssertEqual(SignedUsdFormatter.parse("-7.6"), Decimal(string: "-7.6"))
        XCTAssertEqual(SignedUsdFormatter.parse("\u{2212}7.6"), Decimal(string: "-7.6"))
        XCTAssertEqual(SignedUsdFormatter.parse("+$1,234.50"), Decimal(string: "1234.5"))
        XCTAssertNil(SignedUsdFormatter.parse("garbage"))
    }
}
