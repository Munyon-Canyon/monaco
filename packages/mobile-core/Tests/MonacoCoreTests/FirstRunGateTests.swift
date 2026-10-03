import XCTest

@testable import MonacoCore

final class FirstRunGateTests: XCTestCase {
    private func me(
        displayName: String,
        authState: SessionProfile.AuthState = .created,
        accountStatus: SessionProfile.AccountStatus = .active
    ) -> SessionProfile {
        SessionProfile(
            userID: "user-1", handle: nil, displayName: displayName, photoURL: nil,
            authState: authState, accountStatus: accountStatus, memberWalletAddress: "wallet-1",
            phoneLinked: false, xUsername: nil, handleChangeableAt: nil,
            createdAt: Date(timeIntervalSince1970: 1_759_233_600)
        )
    }

    // MARK: - needsDisplayName

    func testNeedsDisplayName_emptyString() {
        XCTAssertTrue(FirstRunGate.needsDisplayName(""))
    }

    func testNeedsDisplayName_whitespaceOnly() {
        for raw in [" ", "   ", "\t", "\n", " \t\n "] {
            XCTAssertTrue(
                FirstRunGate.needsDisplayName(raw),
                "whitespace-only name \(raw.debugDescription) should count as missing"
            )
        }
    }

    func testNeedsDisplayName_realName() {
        XCTAssertFalse(FirstRunGate.needsDisplayName("Logan Norman"))
    }

    func testNeedsDisplayName_nameWithSurroundingWhitespace() {
        XCTAssertFalse(FirstRunGate.needsDisplayName("  Logan  "))
    }

    func testNeedsDisplayName_singleCharacterAndEmoji() {
        XCTAssertFalse(FirstRunGate.needsDisplayName("L"))
        XCTAssertFalse(FirstRunGate.needsDisplayName("🦈"))
    }

    /// "Member" is only what the boards print for an empty name; `/v1/me` never sends it
    /// for an unnamed account. Someone who typed it must not be asked again on every launch.
    func testNeedsDisplayName_literalMemberIsARealName() {
        XCTAssertFalse(FirstRunGate.needsDisplayName("Member"))
    }

    // MARK: - destination

    func testDestination_noProfileYet_waitsOnTheSession() {
        XCTAssertEqual(FirstRunGate.destination(for: nil), .session)
    }

    func testDestination_emptyName_asksForAName() {
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "")), .nameSetup)
    }

    func testDestination_whitespaceName_asksForAName() {
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "   ")), .nameSetup)
    }

    func testDestination_realName_opensTheTabs() {
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "Logan Norman")), .app)
    }

    /// The decision comes from the decoded `/v1/me` body, so a returning user is routed
    /// by what the server says and a brand new one (`display_name` null) is caught.
    func testDestination_decodedFromMeResponse() throws {
        let named = try SessionProfile(json: Self.wire(displayName: "Ana", authState: "CREATED"))
        XCTAssertEqual(FirstRunGate.destination(for: named), .app)

        let unnamed = try SessionProfile(json: Self.wire(displayName: "", authState: "CREATED"))
        XCTAssertEqual(FirstRunGate.destination(for: unnamed), .nameSetup)

        let phone = try SessionProfile(json: Self.wire(displayName: "Ana", authState: "AWAITING_PHONE"))
        XCTAssertEqual(FirstRunGate.destination(for: phone), .app)
        let completed = try SessionProfile(json: Self.wire(displayName: "", authState: "ONBOARDING_COMPLETED"))
        XCTAssertEqual(FirstRunGate.destination(for: completed), .nameSetup)
    }

    /// The screen saves, `me` comes back named, and the very next evaluation must route
    /// to the tabs — this is what makes the transition happen without a relaunch.
    func testDestination_flipsToAppOnceTheNameIsSaved() {
        let before = me(displayName: "")
        XCTAssertEqual(FirstRunGate.destination(for: before), .nameSetup)
        XCTAssertEqual(FirstRunGate.destination(for: before.withDisplayName("Ana")), .app)
    }

    /// Anything `DisplayNameRules` accepts must get the user through the gate, or first
    /// run could reject a name the server is happy to store and strand the account.
    func testDestination_everyNameTheRulesAcceptOpensTheTabs() throws {
        for raw in ["Ana", "Logan Norman", "J. R. R. T", "x Æ 12", "🦈 shark", "Zoë"] {
            let normalized = try DisplayNameRules.normalize(raw).get()
            XCTAssertEqual(
                FirstRunGate.destination(for: me(displayName: normalized)),
                .app,
                "\(raw.debugDescription) normalizes to \(normalized.debugDescription) and should pass the gate"
            )
        }
    }

    func testDestination_missingNameAsksEvenWhenTheAccountIsFurtherAlong() {
        XCTAssertEqual(
            FirstRunGate.destination(for: me(displayName: "", authState: .onboardingCompleted)), .nameSetup)
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "", authState: .awaitingPhone)), .nameSetup)
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "", accountStatus: .banned)), .nameSetup)
        XCTAssertEqual(FirstRunGate.destination(for: me(displayName: "Ana", authState: .awaitingPhone)), .app)
        XCTAssertEqual(
            FirstRunGate.destination(
                for: me(displayName: "Ana", authState: .onboardingCompleted, accountStatus: .suspended)),
            .app)
        XCTAssertEqual(
            FirstRunGate.destination(for: me(displayName: "Ana", accountStatus: .unknown("frozen"))), .app)
    }

    private static func wire(displayName: String, authState: String) -> Data {
        Data(
            """
            {"id":"u1","display_name":"\(displayName)","auth_state":"\(authState)",\
            "account_status":"active","member_wallet_address":"wallet-1","phone_linked":false,\
            "created_at":"2026-09-30T12:00:00Z"}
            """.utf8)
    }
}
