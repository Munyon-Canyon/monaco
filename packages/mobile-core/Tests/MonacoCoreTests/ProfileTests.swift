import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class BoardPhotoDTOTests: XCTestCase {
    func testBoardDTOs_decodeProfilePhotoUrl() throws {
        let homeURL = try XCTUnwrap(Bundle.module.url(forResource: "home_view", withExtension: "json"))
        let home = try JSONDecoder().decode(HomeViewDTO.self, from: Data(contentsOf: homeURL))
        XCTAssertEqual(
            home.people[0].profilePhotoUrl, "https://example.supabase.co/storage/v1/object/public/avatars/u1/a1.jpg")

        let groupURL = try XCTUnwrap(Bundle.module.url(forResource: "group_view", withExtension: "json"))
        let group = try JSONDecoder().decode(GroupViewDTO.self, from: Data(contentsOf: groupURL))
        XCTAssertEqual(
            group.members[0].profilePhotoUrl, "https://example.supabase.co/storage/v1/object/public/avatars/u1/a1.jpg")
        XCTAssertNil(group.members[1].profilePhotoUrl)
    }

    func testBoardDTOs_missingProfilePhotoUrlDecodesAsNil() throws {
        let json = #"{"rank":1,"userId":"u1","displayName":"A","percentReturn":null,"dollarPnl":"+0.00"}"#

        let row = try JSONDecoder().decode(LeaderboardRowDTO.self, from: Data(json.utf8))

        XCTAssertNil(row.profilePhotoUrl)
    }
}

final class DisplayNameRulesTests: XCTestCase {
    private func scalar(_ value: UInt32) -> String {
        String(Character(Unicode.Scalar(value)!))
    }

    func testNormalize_accepts() {
        let cases: [(String, String)] = [
            ("Logan", "Logan"),
            ("  Logan \n", "Logan"),
            ("Logan    Norman", "Logan Norman"),
            ("Logan" + scalar(0x00A0) + scalar(0x3000) + "Norman", "Logan Norman"),
            ("L", "L"),
            ("2049", "2049"),
            ("O'Brien-Smith Jr.", "O'Brien-Smith Jr."),
            ("Jose" + scalar(0x0301), "Jos" + scalar(0x00E9)),
            ("Ana " + scalar(0x1F680), "Ana " + scalar(0x1F680)),
            (String(repeating: scalar(0x00E9), count: 32), String(repeating: scalar(0x00E9), count: 32)),
        ]
        for (raw, want) in cases {
            XCTAssertEqual(try? DisplayNameRules.normalize(raw).get(), want, "raw: \(raw.debugDescription)")
        }
    }

    func testNormalize_rejects() {
        let cases: [(String, DisplayNameValidationError)] = [
            ("", .required),
            ("   \n", .required),
            (String(repeating: "a", count: 33), .tooLong),
            (String(repeating: scalar(0x00E9), count: 33), .tooLong),
            ("Lo\ngan", .invalidCharacters),
            ("Lo\tgan", .invalidCharacters),
            ("Lo" + scalar(0x0007) + "gan", .invalidCharacters),
            ("Lo" + scalar(0x200B) + "gan", .invalidCharacters),
            ("Lo" + scalar(0x200D) + "gan", .invalidCharacters),
            (scalar(0x202E) + "Logan", .invalidCharacters),
            (scalar(0x3164), .invalidCharacters),
            ("Lo" + scalar(0x2800) + "gan", .invalidCharacters),
            ("Logan" + scalar(0xE000), .invalidCharacters),
            ("...", .needsLetterOrNumber),
            (scalar(0x1F680), .needsLetterOrNumber),
        ]
        for (raw, want) in cases {
            switch DisplayNameRules.normalize(raw) {
            case .success(let value):
                XCTFail("\(raw.debugDescription) accepted as \(value.debugDescription), want \(want)")
            case .failure(let error):
                XCTAssertEqual(error, want, "raw: \(raw.debugDescription)")
            }
        }
    }

    func testNormalize_rejectsStackedCombiningMarks() {
        let zalgo = "Lox" + scalar(0x0301) + scalar(0x0302) + scalar(0x0303) + "gan"
        XCTAssertEqual(
            DisplayNameRules.validationMessage(for: zalgo), DisplayNameValidationError.invalidCharacters.message)
    }

    func testValidationMessage_matchesServerCopy() {
        XCTAssertNil(DisplayNameRules.validationMessage(for: "Logan"))
        XCTAssertEqual(DisplayNameRules.validationMessage(for: ""), "Display name is required.")
        XCTAssertEqual(
            DisplayNameRules.validationMessage(for: String(repeating: "a", count: 40)),
            "Display name must be 32 characters or fewer."
        )
    }
}

final class AvatarInitialsTests: XCTestCase {
    func testInitials() {
        XCTAssertEqual(AvatarInitials.from("Logan Norman"), "LN")
        XCTAssertEqual(AvatarInitials.from("logan"), "L")
        XCTAssertEqual(AvatarInitials.from("Mary Ann van Dyke"), "MD")
        XCTAssertEqual(AvatarInitials.from("  ana  "), "A")
        XCTAssertEqual(AvatarInitials.from("O'Brien"), "O")
        XCTAssertEqual(AvatarInitials.from("..."), "")
        XCTAssertEqual(AvatarInitials.from(""), "")
    }

    func testMemberSince_usesViewerTimeZone() {
        // 2026-09-01T02:00Z is still August 31 in Los Angeles.
        let date = Date(timeIntervalSince1970: 1_788_228_000)
        let locale = Locale(identifier: "en_US")

        XCTAssertEqual(
            MemberSinceFormatter.format(date, timeZone: TimeZone(identifier: "UTC")!, locale: locale),
            "Member since Sep 2026")
        XCTAssertEqual(
            MemberSinceFormatter.format(date, timeZone: TimeZone(identifier: "America/Los_Angeles")!, locale: locale),
            "Member since Aug 2026")
    }
}
