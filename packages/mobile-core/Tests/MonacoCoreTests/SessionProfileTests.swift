import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class SessionProfileTests: XCTestCase {
    func testFullMappingFromGeneratedMe() throws {
        let createdAt = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-09-30T12:00:00Z"))
        let changeableAt = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-30T12:00:00Z"))
        let me = Components.Schemas.Me(
            id: "01890a5d-ac96-774b-bcce-b302099a8058",
            handle: "kaicenat",
            displayName: "Kai Cenat",
            photoUrl: "https://cdn.example.com/photos/kai.jpg",
            authState: .onboardingCompleted,
            accountStatus: .active,
            memberWalletAddress: "wallet-1",
            phoneLinked: true,
            xUsername: "kaicenat",
            handleChangeableAt: changeableAt,
            createdAt: createdAt
        )

        let profile = SessionProfile(me)

        XCTAssertEqual(profile.userID, me.id)
        XCTAssertEqual(profile.handle, "kaicenat")
        XCTAssertEqual(profile.displayName, "Kai Cenat")
        XCTAssertEqual(profile.photoURL, URL(string: "https://cdn.example.com/photos/kai.jpg"))
        XCTAssertEqual(profile.authState, .onboardingCompleted)
        XCTAssertEqual(profile.accountStatus, .active)
        XCTAssertEqual(profile.memberWalletAddress, "wallet-1")
        XCTAssertTrue(profile.phoneLinked)
        XCTAssertEqual(profile.xUsername, "kaicenat")
        XCTAssertEqual(profile.handleChangeableAt, changeableAt)
        XCTAssertEqual(profile.createdAt, createdAt)
    }

    func testMappingCoversEveryNamedState() {
        let createdAt = Date(timeIntervalSince1970: 1_759_233_600)
        func me(auth: Components.Schemas.AuthState, status: Components.Schemas.AccountStatus) -> Components.Schemas.Me {
            Components.Schemas.Me(
                id: "01890a5d-ac96-774b-bcce-b302099a8058",
                displayName: "",
                authState: auth,
                accountStatus: status,
                memberWalletAddress: "wallet-1",
                phoneLinked: false,
                createdAt: createdAt
            )
        }

        XCTAssertEqual(SessionProfile(me(auth: .created, status: .active)).authState, .created)
        XCTAssertEqual(SessionProfile(me(auth: .awaitingPhone, status: .suspended)).authState, .awaitingPhone)
        XCTAssertEqual(SessionProfile(me(auth: .awaitingSocials, status: .banned)).authState, .awaitingSocials)
        XCTAssertEqual(SessionProfile(me(auth: .created, status: .suspended)).accountStatus, .suspended)
        XCTAssertEqual(SessionProfile(me(auth: .created, status: .banned)).accountStatus, .banned)
    }

    func testNullHandleOmitsTheUnsetFields() throws {
        let profile = try SessionProfile(json: Data(Self.minimumJSON.utf8))

        XCTAssertNil(profile.handle)
        XCTAssertEqual(profile.displayName, "")
        XCTAssertNil(profile.photoURL)
        XCTAssertEqual(profile.authState, .created)
        XCTAssertEqual(profile.accountStatus, .active)
        XCTAssertFalse(profile.phoneLinked)
        XCTAssertNil(profile.xUsername)
        XCTAssertNil(profile.handleChangeableAt)
        XCTAssertEqual(profile.userID, "01890a5d-ac96-774b-bcce-b302099a8058")
    }

    func testUnknownStatesDegrade() throws {
        let json = Self.minimumJSON
            .replacingOccurrences(of: "\"CREATED\"", with: "\"AWAITING_VIDEO\"")
            .replacingOccurrences(of: "\"active\"", with: "\"frozen\"")

        let profile = try SessionProfile(json: Data(json.utf8))

        XCTAssertEqual(profile.authState, .unknown("AWAITING_VIDEO"))
        XCTAssertEqual(profile.accountStatus, .unknown("frozen"))
    }

    func testDateParseFailureMapsToDecoding() {
        let json = Self.minimumJSON.replacingOccurrences(of: "2026-09-30T12:00:00Z", with: "yesterday")

        XCTAssertThrowsError(try SessionProfile(json: Data(json.utf8))) { error in
            guard case APIError.decoding("created_at") = error else {
                XCTFail("expected .decoding, got \(error)")
                return
            }
        }
    }

    func testHandleChangeableAtParseFailureMapsToDecoding() {
        let json = Self.minimumJSON.replacingOccurrences(
            of: "\"phone_linked\":false",
            with: "\"phone_linked\":false,\"handle_changeable_at\":\"yesterday\""
        )

        XCTAssertThrowsError(try SessionProfile(json: Data(json.utf8))) { error in
            guard case APIError.decoding("handle_changeable_at") = error else {
                XCTFail("expected .decoding, got \(error)")
                return
            }
        }
    }

    func testFractionalCreatedAtMaps() throws {
        let json = Self.minimumJSON.replacingOccurrences(
            of: "2026-09-30T12:00:00Z",
            with: "2026-09-30T12:00:00.123456Z"
        )
        let whole = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-09-30T12:00:00Z"))

        let profile = try SessionProfile(json: Data(json.utf8))

        XCTAssertEqual(profile.createdAt.timeIntervalSince(whole), 0.123456, accuracy: 0.0000005)
    }

    func testBrokenJSONMapsToDecoding() {
        XCTAssertThrowsError(try SessionProfile(json: Data("{}".utf8))) { error in
            guard case APIError.decoding = error else {
                XCTFail("expected .decoding, got \(error)")
                return
            }
        }
    }

    func testReplacingCopiesTheProfileFieldsAndKeepsAnOlderCreationDate() {
        let createdAt = Date(timeIntervalSince1970: 1_700_000_000)
        let profile = SessionProfile(
            userID: "old-user", handle: "old", displayName: "Old", photoURL: URL(string: "https://cdn.test/old"),
            authState: .created, accountStatus: .active, memberWalletAddress: "old-wallet", phoneLinked: false,
            xUsername: nil, handleChangeableAt: nil, createdAt: createdAt
        )
        let dto = MeDTO(
            userId: "new-user", displayName: "New", memberWalletAddress: "new-wallet",
            profilePhotoUrl: "avatars/new.jpg"
        )

        let replaced = profile.replacing(from: dto)

        XCTAssertEqual(replaced.userID, "new-user")
        XCTAssertEqual(replaced.displayName, "New")
        XCTAssertEqual(replaced.memberWalletAddress, "new-wallet")
        XCTAssertNil(replaced.photoURL)
        XCTAssertEqual(replaced.createdAt, createdAt)
    }

    func testReplacingTakesTheCreationDateWhenTheDTOHasOne() {
        let createdAt = Date(timeIntervalSince1970: 1_800_000_000)
        let profile = SessionProfile(
            userID: "user", handle: nil, displayName: "Old", photoURL: nil, authState: .created,
            accountStatus: .active, memberWalletAddress: "wallet", phoneLinked: false, xUsername: nil,
            handleChangeableAt: nil, createdAt: .distantPast
        )

        let replaced = profile.replacing(
            from: MeDTO(
                userId: "user", displayName: "New", memberWalletAddress: "wallet", createdAt: createdAt
            ))

        XCTAssertEqual(replaced.createdAt, createdAt)
    }

    private static let minimumJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","display_name":"","auth_state":"CREATED",\
        "account_status":"active","member_wallet_address":"wallet-1","phone_linked":false,\
        "created_at":"2026-09-30T12:00:00Z"}
        """
}
