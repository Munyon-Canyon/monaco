import Foundation
import MonacoCore
import XCTest

@MainActor
final class PushPrePromptTests: XCTestCase {
    private var registered = 0

    func testThePolicyShowsOnlyAnUndecidedStatusThatWasNotShownBefore() {
        let table: [(status: PushAuthorization, alreadyShown: Bool, shows: Bool)] = [
            (.notDetermined, false, true),
            (.notDetermined, true, false),
            (.denied, false, false),
            (.denied, true, false),
            (.authorized, false, false),
            (.authorized, true, false),
        ]

        XCTAssertEqual(Set(table.map(\.status)), Set(PushAuthorization.allCases))
        XCTAssertEqual(table.count, PushAuthorization.allCases.count * 2)
        for row in table {
            XCTAssertEqual(
                PushPrePromptPolicy.shouldShow(status: row.status, alreadyShown: row.alreadyShown), row.shows,
                "\(row.status), shown before: \(row.alreadyShown)")
        }
    }

    func testProvisionalAndEphemeralCountAsAuthorized() {
        let expected: [PushAuthorizationStatus: PushAuthorization] = [
            .notDetermined: .notDetermined,
            .denied: .denied,
            .authorized: .authorized,
            .provisional: .authorized,
            .ephemeral: .authorized,
        ]

        XCTAssertEqual(Set(expected.keys), Set(PushAuthorizationStatus.allCases))
        for (status, authorization) in expected {
            XCTAssertEqual(PushAuthorization(status), authorization, "\(status)")
        }
    }

    func testTheFirstJoinShowsTheSheetAndLeavesTheSystemPromptAlone() async throws {
        let setup = try makeSetup()
        XCTAssertFalse(setup.prompt.isPresented)

        await setup.prompt.noteCabalJoined()

        XCTAssertTrue(setup.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 0)
        XCTAssertEqual(registered, 0)
    }

    func testADecidedStatusNeverShowsTheSheet() async throws {
        for status in [PushAuthorization.denied, .authorized] {
            let setup = try makeSetup(status: status)

            await setup.prompt.noteCabalJoined()

            XCTAssertFalse(setup.prompt.isPresented, "\(status)")
        }
    }

    func testNotNowHidesTheSheetAndItNeverShowsAgainEvenAfterARelaunch() async throws {
        let setup = try makeSetup()
        await setup.prompt.noteCabalJoined()

        setup.prompt.notNow()

        XCTAssertFalse(setup.prompt.isPresented)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteCabalJoined()
        XCTAssertFalse(setup.prompt.isPresented)
        let relaunched = try makeSetup(defaults: setup.defaults)
        await relaunched.prompt.noteCabalJoined()
        XCTAssertFalse(relaunched.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 0)
        XCTAssertEqual(registered, 0)
    }

    func testTurnOnAsksOnceRegistersOnceAndTheSheetStaysGone() async throws {
        let setup = try makeSetup(grants: true)
        await setup.prompt.noteCabalJoined()

        await setup.prompt.turnOn()

        XCTAssertFalse(setup.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 1)
        XCTAssertEqual(registered, 1)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteCabalJoined()
        XCTAssertFalse(setup.prompt.isPresented)
    }

    func testADeniedPromptRegistersNothingAndTheSheetStaysGone() async throws {
        let setup = try makeSetup(grants: false)
        await setup.prompt.noteCabalJoined()

        await setup.prompt.turnOn()

        XCTAssertFalse(setup.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 1)
        XCTAssertEqual(registered, 0)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteCabalJoined()
        XCTAssertFalse(setup.prompt.isPresented)
    }

    private struct Setup {
        let prompt: PushPrePrompt
        let authorization: FakeAuthorization
        let defaults: UserDefaults
    }

    private func makeSetup(
        status: PushAuthorization = .notDetermined,
        grants: Bool = true,
        defaults existing: UserDefaults? = nil
    ) throws -> Setup {
        let defaults = try existing ?? makeDefaults()
        let authorization = FakeAuthorization(status: status, grants: grants)
        let prompt = PushPrePrompt(authorization: authorization, defaults: defaults) { [self] in registered += 1 }
        return Setup(prompt: prompt, authorization: authorization, defaults: defaults)
    }

    private func makeDefaults() throws -> UserDefaults {
        let suite = "PushPrePromptTests.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        addTeardownBlock { UserDefaults(suiteName: suite)?.removePersistentDomain(forName: suite) }
        return defaults
    }
}

private actor FakeAuthorization: NotificationAuthorizing {
    private var current: PushAuthorization
    private let grants: Bool
    private(set) var requests = 0

    init(status: PushAuthorization, grants: Bool) {
        current = status
        self.grants = grants
    }

    func status() -> PushAuthorization { current }

    func request() -> Bool {
        requests += 1
        if current == .notDetermined { current = grants ? .authorized : .denied }
        return current == .authorized
    }
}
