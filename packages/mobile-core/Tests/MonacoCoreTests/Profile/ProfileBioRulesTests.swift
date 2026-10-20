import MonacoCore
import XCTest

final class ProfileBioRulesTests: XCTestCase {
    func testABioOfExactlyTheLimitPasses() {
        let bio = String(repeating: "a", count: ProfileBioRules.maxScalars)
        XCTAssertNil(ProfileBioRules.validationMessage(for: bio))
        XCTAssertEqual(ProfileBioRules.remaining(bio), 0)
    }

    func testOneScalarOverTheLimitFails() {
        let bio = String(repeating: "a", count: ProfileBioRules.maxScalars + 1)
        XCTAssertEqual(ProfileBioRules.validationMessage(for: bio), "Bio must be 160 characters or fewer.")
        XCTAssertEqual(ProfileBioRules.remaining(bio), -1)
    }

    func testTheLimitCountsScalarsNotGraphemes() {
        let family = "👨‍👩‍👧"
        XCTAssertEqual(family.count, 1)
        XCTAssertEqual(family.unicodeScalars.count, 5)
        let bio = String(repeating: family, count: 32)
        XCTAssertEqual(ProfileBioRules.scalarCount(bio), 160)
        XCTAssertNil(ProfileBioRules.validationMessage(for: bio))
        XCTAssertNotNil(ProfileBioRules.validationMessage(for: bio + "a"))
    }

    func testNewlinesAreKeptAndOtherControlCharactersAreRejected() {
        XCTAssertNil(ProfileBioRules.validationMessage(for: "Building in public.\nNYC"))
        XCTAssertEqual(ProfileBioRules.normalized("Building in public.\nNYC"), "Building in public.\nNYC")
        XCTAssertNotNil(ProfileBioRules.validationMessage(for: "tab\there"))
        XCTAssertNotNil(ProfileBioRules.validationMessage(for: "carriage\rreturn"))
    }

    func testTheBioIsTrimmedBeforeItIsCounted() {
        let padded = "  \n" + String(repeating: "a", count: 160) + "\n  "
        XCTAssertNil(ProfileBioRules.validationMessage(for: padded))
        XCTAssertEqual(ProfileBioRules.scalarCount(padded), 160)
    }

    func testABlankBioNormalizesToEmpty() {
        XCTAssertEqual(ProfileBioRules.normalized(" \n "), "")
        XCTAssertNil(ProfileBioRules.validationMessage(for: ""))
    }
}
