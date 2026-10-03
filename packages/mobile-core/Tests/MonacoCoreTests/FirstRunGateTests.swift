import XCTest

@testable import MonacoCore

final class FirstRunGateTests: XCTestCase {
    private typealias AuthState = SessionProfile.AuthState
    private typealias AccountStatus = SessionProfile.AccountStatus

    private static let statuses: [AccountStatus] = [.active, .suspended, .banned, .unknown("frozen")]
    private static let authStates: [AuthState] = [
        .created, .awaitingPhone, .awaitingSocials, .onboardingCompleted, .unknown("AWAITING_KYC"),
    ]
    private static let handles: [String?] = [nil, "ana"]
    private static let cursors: [OnboardingCursor] = [.start, .socials, .finished]

    private func me(
        handle: String?, authState: AuthState, accountStatus: AccountStatus
    ) -> SessionProfile {
        SessionProfile(
            userID: "user-1", handle: handle, displayName: "", photoURL: nil,
            authState: authState, accountStatus: accountStatus, memberWalletAddress: "wallet-1",
            phoneLinked: false, xUsername: nil, handleChangeableAt: nil,
            createdAt: Date()
        )
    }

    private func route(
        handle: String? = "ana", _ authState: AuthState, _ accountStatus: AccountStatus = .active,
        cursor: OnboardingCursor = .start
    ) -> FirstRunDestination {
        FirstRunGate.destination(
            for: me(handle: handle, authState: authState, accountStatus: accountStatus),
            onboardingCursor: cursor)
    }

    private static func expected(
        handle: String?, authState: AuthState, accountStatus: AccountStatus, cursor: OnboardingCursor
    ) -> FirstRunDestination {
        switch (accountStatus, handle, authState, cursor) {
        case (.banned, _, _, _): .restricted(.banned)
        case (_, nil, _, _): .handle
        case (_, _, .created, _): .phone
        case (_, _, .awaitingSocials, .socials), (_, _, .awaitingPhone, .socials): .socials
        case (.suspended, _, _, _): .app(restricted: true)
        default: .app(restricted: false)
        }
    }

    func testDestination_everyStatusAuthStateHandleAndCursor() {
        var checked = 0
        for status in Self.statuses {
            for authState in Self.authStates {
                for handle in Self.handles {
                    for cursor in Self.cursors {
                        XCTAssertEqual(
                            FirstRunGate.destination(
                                for: me(handle: handle, authState: authState, accountStatus: status),
                                onboardingCursor: cursor),
                            Self.expected(
                                handle: handle, authState: authState, accountStatus: status, cursor: cursor),
                            "status \(status), auth \(authState), handle \(handle ?? "nil"), cursor \(cursor)"
                        )
                        checked += 1
                    }
                }
            }
        }
        XCTAssertEqual(checked, 4 * 5 * 2 * 3)
    }

    func testDestination_noProfileYet_waitsOnTheSession() {
        for cursor in Self.cursors {
            XCTAssertEqual(FirstRunGate.destination(for: nil, onboardingCursor: cursor), .session)
        }
    }

    func testDestination_bannedOutranksEveryOnboardingStep() {
        XCTAssertEqual(route(handle: nil, .created, .banned), .restricted(.banned))
        XCTAssertEqual(route(.created, .banned), .restricted(.banned))
        XCTAssertEqual(route(.awaitingSocials, .banned, cursor: .socials), .restricted(.banned))
        XCTAssertEqual(route(.onboardingCompleted, .banned), .restricted(.banned))
    }

    func testDestination_noHandle_isOnlyTheHandleStep() {
        XCTAssertEqual(route(handle: nil, .created), .handle)
        XCTAssertEqual(route(handle: nil, .onboardingCompleted), .handle)
        XCTAssertEqual(route(handle: nil, .awaitingSocials, .suspended, cursor: .socials), .handle)
        XCTAssertEqual(route(handle: nil, .unknown("NEW"), .unknown("frozen")), .handle)
    }

    func testDestination_createdWithHandle_resumesAtThePhoneStep() {
        XCTAssertEqual(route(.created), .phone)
        XCTAssertEqual(route(.created, .suspended), .phone)
        XCTAssertEqual(route(.created, cursor: .socials), .phone)
    }

    func testDestination_socialsOnlyRightAfterThePhoneStepThisLaunch() {
        XCTAssertEqual(route(.awaitingSocials, cursor: .socials), .socials)
        XCTAssertEqual(route(.awaitingPhone, cursor: .socials), .socials)
        XCTAssertEqual(route(.onboardingCompleted, cursor: .socials), .app(restricted: false))
    }

    func testDestination_relaunchAwaitingSocialsOpensTheApp() {
        XCTAssertEqual(route(.awaitingSocials, cursor: .start), .app(restricted: false))
        XCTAssertEqual(route(.awaitingPhone, cursor: .start), .app(restricted: false))
        XCTAssertEqual(route(.awaitingSocials, cursor: .finished), .app(restricted: false))
    }

    func testDestination_suspendedGetsTheReadOnlyApp() {
        XCTAssertEqual(route(.onboardingCompleted, .suspended), .app(restricted: true))
        XCTAssertEqual(route(.awaitingPhone, .suspended), .app(restricted: true))
        XCTAssertEqual(route(.unknown("LATER"), .suspended), .app(restricted: true))
    }

    func testDestination_unknownStatesEnterTheApp() {
        XCTAssertEqual(route(.unknown("AWAITING_KYC")), .app(restricted: false))
        XCTAssertEqual(route(.onboardingCompleted, .unknown("frozen")), .app(restricted: false))
        XCTAssertEqual(route(.unknown("AWAITING_KYC"), .unknown("frozen")), .app(restricted: false))
    }

    func testDestination_emptyDisplayNameDoesNotGate() {
        XCTAssertEqual(route(.onboardingCompleted), .app(restricted: false))
    }

    func testDestination_renamingChangesOnlyTheNameAndNeverTheRoute() {
        for status in Self.statuses {
            for authState in Self.authStates {
                for handle in Self.handles {
                    let before = me(handle: handle, authState: authState, accountStatus: status)
                    let renamed = before.withDisplayName("Ana")
                    XCTAssertEqual(renamed.displayName, "Ana")
                    var expected = before
                    expected.displayName = "Ana"
                    XCTAssertEqual(renamed, expected, "rename touched more than the name")
                    for cursor in Self.cursors {
                        XCTAssertEqual(
                            FirstRunGate.destination(for: renamed, onboardingCursor: cursor),
                            FirstRunGate.destination(for: before, onboardingCursor: cursor),
                            "renaming moved the gate: \(status) \(authState) \(handle ?? "nil") \(cursor)")
                    }
                }
            }
        }
    }

    func testDestination_decodedFromMeResponse() throws {
        let cases: [(handle: String?, authState: String, status: String, want: FirstRunDestination)] = [
            (nil, "CREATED", "active", .handle),
            ("ana", "CREATED", "active", .phone),
            ("ana", "AWAITING_PHONE", "active", .app(restricted: false)),
            ("ana", "ONBOARDING_COMPLETED", "suspended", .app(restricted: true)),
            ("ana", "ONBOARDING_COMPLETED", "banned", .restricted(.banned)),
            ("ana", "SOMETHING_NEW", "frozen", .app(restricted: false)),
        ]
        for item in cases {
            let profile = try SessionProfile(
                json: Self.wire(handle: item.handle, authState: item.authState, status: item.status))
            XCTAssertEqual(
                FirstRunGate.destination(for: profile, onboardingCursor: .start), item.want,
                "\(item.handle ?? "nil") \(item.authState) \(item.status)")
        }
    }

    func testCursor_phoneThenSocialsWalksToTheApp() {
        var cursor = OnboardingCursor.start
        let created = me(handle: "ana", authState: .created, accountStatus: .active)
        XCTAssertEqual(FirstRunGate.destination(for: created, onboardingCursor: cursor), .phone)

        cursor = cursor.advanced(past: .phone)
        let awaitingSocials = me(handle: "ana", authState: .awaitingSocials, accountStatus: .active)
        XCTAssertEqual(FirstRunGate.destination(for: awaitingSocials, onboardingCursor: cursor), .socials)

        cursor = cursor.advanced(past: .socials)
        XCTAssertEqual(
            FirstRunGate.destination(for: awaitingSocials, onboardingCursor: cursor), .app(restricted: false))
    }

    func testCursor_stepsWithoutACursorLeaveItAlone() {
        for cursor in Self.cursors {
            for step: FirstRunDestination in [
                .session, .handle, .restricted(.banned), .app(restricted: false), .app(restricted: true),
            ] {
                XCTAssertEqual(cursor.advanced(past: step), cursor, "\(cursor) past \(step)")
            }
        }
    }

    func testCopy_passesTheMainFlowAudit() {
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(OnboardingCopy.auditedStrings))
        for string in OnboardingCopy.auditedStrings {
            XCTAssertTrue(MainFlowCopyManifest.mainFlowStrings.contains(string), string)
        }
    }

    private static func wire(handle: String?, authState: String, status: String) -> Data {
        let handleField = handle.map { "\"handle\":\"\($0)\"," } ?? ""
        return Data(
            """
            {"id":"u1",\(handleField)"display_name":"","auth_state":"\(authState)",\
            "account_status":"\(status)","member_wallet_address":"wallet-1","phone_linked":false,\
            "created_at":"2026-09-30T12:00:00Z"}
            """.utf8)
    }
}
