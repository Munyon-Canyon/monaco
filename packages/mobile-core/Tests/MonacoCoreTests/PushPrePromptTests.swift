import Foundation
import XCTest

@testable import MonacoCore

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

    func testTheFirstVoteShowsTheSheetAndLeavesTheSystemPromptAlone() async throws {
        let setup = try makeSetup()
        XCTAssertFalse(setup.prompt.isPresented)

        await setup.prompt.noteFirstVote(sample: sample)

        XCTAssertTrue(setup.prompt.isPresented)
        XCTAssertEqual(setup.prompt.sample, sample)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 0)
        XCTAssertEqual(registered, 0)
    }

    func testADecidedStatusNeverShowsTheSheet() async throws {
        for status in [PushAuthorization.denied, .authorized] {
            let setup = try makeSetup(status: status)

            await setup.prompt.noteFirstVote(sample: sample)

            XCTAssertFalse(setup.prompt.isPresented, "\(status)")
        }
    }

    func testNotNowHidesTheSheetAndItNeverShowsAgainEvenAfterARelaunch() async throws {
        let setup = try makeSetup()
        await setup.prompt.noteFirstVote(sample: sample)

        setup.prompt.notNow()

        XCTAssertFalse(setup.prompt.isPresented)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteFirstVote(sample: sample)
        XCTAssertFalse(setup.prompt.isPresented)
        let relaunched = try makeSetup(defaults: setup.defaults)
        await relaunched.prompt.noteFirstVote(sample: sample)
        XCTAssertFalse(relaunched.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 0)
        XCTAssertEqual(registered, 0)
    }

    func testTurnOnAsksOnceRegistersOnceAndTheSheetStaysGone() async throws {
        let setup = try makeSetup(grants: true)
        await setup.prompt.noteFirstVote(sample: sample)

        await setup.prompt.turnOn()

        XCTAssertFalse(setup.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 1)
        XCTAssertEqual(registered, 1)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteFirstVote(sample: sample)
        XCTAssertFalse(setup.prompt.isPresented)
    }

    func testADeniedPromptRegistersNothingAndTheSheetStaysGone() async throws {
        let setup = try makeSetup(grants: false)
        await setup.prompt.noteFirstVote(sample: sample)

        await setup.prompt.turnOn()

        XCTAssertFalse(setup.prompt.isPresented)
        let requests = await setup.authorization.requests
        XCTAssertEqual(requests, 1)
        XCTAssertEqual(registered, 0)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteFirstVote(sample: sample)
        XCTAssertFalse(setup.prompt.isPresented)
    }

    func testASecondVoteDoesNotShowTheSheetAgain() async throws {
        let setup = try makeSetup()
        await setup.prompt.noteFirstVote(sample: sample)
        setup.prompt.notNow()

        await setup.prompt.noteFirstVote(sample: sample)

        XCTAssertFalse(setup.prompt.isPresented)
    }

    func testJoiningACabalNeverShowsTheSheetInAnyState() async throws {
        for status in PushAuthorization.allCases {
            let setup = try makeSetup(status: status)

            await setup.prompt.noteCabalJoined()

            XCTAssertFalse(setup.prompt.isPresented, "\(status)")
            XCTAssertNil(setup.prompt.sample)
        }
    }

    func testDeliverQuietlyAsksForProvisionalRegistersAndTheSheetStaysGone() async throws {
        let setup = try makeSetup(grants: true)
        await setup.prompt.noteFirstVote(sample: sample)

        await setup.prompt.deliverQuietly()

        XCTAssertFalse(setup.prompt.isPresented)
        let provisional = await setup.authorization.provisionalRequests
        let requests = await setup.authorization.requests
        XCTAssertEqual(provisional, 1)
        XCTAssertEqual(requests, 0)
        XCTAssertEqual(registered, 1)
        XCTAssertTrue(setup.defaults.bool(forKey: "pushPrePromptShown"))
        await setup.prompt.noteFirstVote(sample: sample)
        XCTAssertFalse(setup.prompt.isPresented)
    }

    func testDeliverQuietlyRegistersNothingWhenRefused() async throws {
        let setup = try makeSetup(grants: false)
        await setup.prompt.noteFirstVote(sample: sample)

        await setup.prompt.deliverQuietly()

        XCTAssertFalse(setup.prompt.isPresented)
        XCTAssertEqual(registered, 0)
    }

    func testTheDefaultProvisionalRequestFallsBackToRequest() async {
        let plain = PlainAuthorization()
        let granted = await plain.requestProvisional()
        XCTAssertTrue(granted)
    }

    func testTheSampleAfterAVote() {
        let cal = Calendar(identifier: .gregorian)
        var utc = cal
        utc.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        let closes = Date(timeIntervalSince1970: 1_792_662_780)
        let fill = ProposalSwap(
            .init(
                swapId: "s", status: .confirmed, failureCode: nil, failureMessage: nil, txSignature: nil,
                retryable: false, usdcMicros: nil, tokenAmount: 182_000_000, tokenDecimals: 8,
                priceMicros: 219_400_000, feeMicros: nil, filledAt: closes))
        let passed = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)
        let failed = ProposalTally(yes: 2, no: 3, voters: 8, needed: 5)

        func make(
            _ symbol: String, _ status: ProposalStatus, _ tally: ProposalTally, swap: ProposalSwap? = nil,
            units: String? = nil, sell: Bool = false, usdc: Int64? = 400_000_000
        ) -> PushSample {
            .afterVote(
                symbol: symbol, status: status, tally: tally, isSell: sell, usdcMicros: usdc, closesAt: closes,
                swap: swap, pendingUnits: units, calendar: utc)
        }

        XCTAssertEqual(
            make("NVDAx", .executed, passed, swap: fill),
            PushSample(title: "NVDA passed 5–1", body: "Bought 1.82 sh at $219.40."))
        XCTAssertEqual(make("NVDAx", .passed, passed, units: "1.82").body, "Buying 1.82 sh now.")
        XCTAssertEqual(make("NVDAx", .passed, passed, sell: true).body, "Selling now.")
        XCTAssertEqual(
            make("TSLA", .failed, failed), PushSample(title: "TSLA failed 2–3", body: "No trade."))
        XCTAssertEqual(
            make("NVDAx", .open, passed),
            PushSample(title: "Buy NVDA $400 · closes 9:53 AM", body: "We'll tell you how it ends."))
        XCTAssertEqual(make("NVDAx", .open, passed, sell: true, usdc: nil).title, "Sell NVDA · closes 9:53 AM")
    }

    private let sample = PushSample(title: "NVDA passed 5–1", body: "Bought 1.82 sh at $219.40.")

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
    private(set) var provisionalRequests = 0

    init(status: PushAuthorization, grants: Bool) {
        current = status
        self.grants = grants
    }

    func status() -> PushAuthorization { current }

    func requestProvisional() -> Bool {
        provisionalRequests += 1
        current = grants ? .authorized : .denied
        return current == .authorized
    }

    func request() -> Bool {
        requests += 1
        if current == .notDetermined { current = grants ? .authorized : .denied }
        return current == .authorized
    }
}

private struct PlainAuthorization: NotificationAuthorizing {
    func status() async -> PushAuthorization { .notDetermined }
    func request() async -> Bool { true }
}
