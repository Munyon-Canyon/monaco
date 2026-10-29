import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class OwnProfileModelTests: XCTestCase {
    private static let userID = "01890a5d-ac96-774b-bcce-b302099a8070"

    private typealias Standing = Components.Schemas.PublicProfile.StandingPayload

    func testARankedStandingReadsTheRateTheCallsAndTheRank() async throws {
        let (model, _, _) = try make([.profile(.sampleOwn)])

        await model.load()

        XCTAssertEqual(
            model.standing, .ranked(ratePercent: 78, right: 7, resolved: 9, rank: 8, rankedCount: 24))
        let line = try XCTUnwrap(model.standing).line
        XCTAssertEqual(line.plain, "78% right calls this month · 7 of 9 · #8 of 24")
        XCTAssertEqual(line.runs.filter(\.isStrong).map(\.text), ["78%", "7 of 9", "#8"])
    }

    func testAnUnrankedStandingReadsTheResolvedCallsAndHowManyMoreToRank() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.standing = .sampleUnranked

        let (model, _, _) = try make([.profile(user)])
        await model.load()

        XCTAssertEqual(model.standing, .unranked(resolved: 3, moreToRank: 2))
        let line = try XCTUnwrap(model.standing).line
        XCTAssertEqual(line.plain, "3 resolved this month · 2 more to rank")
        XCTAssertEqual(line.runs.filter(\.isStrong).map(\.text), ["3", "2"])
    }

    func testNoEligibleVotesHideTheStanding() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.standing = nil

        let (model, _, _) = try make([.profile(user)])
        await model.load()

        XCTAssertNil(model.standing)
    }

    func testTheCardNamesTheMemberTheHandleAndWhenTheyJoined() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.displayName = " Andres "
        user.bio = "Building Monaco. Long NVDA, short sleep."

        let (model, _, _) = try make([.profile(user)])
        XCTAssertNil(model.card)
        await model.load()

        let card = try XCTUnwrap(model.card)
        XCTAssertEqual(card.displayName, "Andres")
        XCTAssertEqual(card.handle, "andres")
        XCTAssertEqual(card.bio, "Building Monaco. Long NVDA, short sleep.")
        XCTAssertTrue(card.identityLine.hasPrefix("@andres · joined "), card.identityLine)
        XCTAssertEqual(
            ProfileCopy.identityLine(
                handle: "andres", joinedAt: Date(timeIntervalSince1970: 1_755_000_000),
                timeZone: TimeZone(secondsFromGMT: 0) ?? .current, locale: Locale(identifier: "en_US_POSIX")),
            "@andres · joined Aug 2025")
        XCTAssertEqual(
            ProfileCopy.identityLine(
                handle: nil, joinedAt: Date(timeIntervalSince1970: 1_755_000_000),
                timeZone: TimeZone(secondsFromGMT: 0) ?? .current, locale: Locale(identifier: "en_US_POSIX")),
            "joined Aug 2025")
    }

    func testTheCountsLineBoldsEachCount() async throws {
        let (model, _, _) = try make([.profile(.sampleOwn)])

        await model.load()

        let counts = try XCTUnwrap(model.counts)
        XCTAssertEqual(counts, OwnProfileCounts(followers: 12, following: 8, cabals: 3))
        XCTAssertEqual(counts.line.plain, "12 followers · 8 following · 3 cabals")
        XCTAssertEqual(counts.line.runs.filter(\.isStrong).map(\.text), ["12", "8", "3"])
    }

    func testAGainALossAndAFlatMonthKeepTheirSignAndToneRole() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.pnl1mMicros = 241_100_000
        let (gain, _, _) = try make([.profile(user)])
        await gain.load()
        XCTAssertEqual(gain.pnl1M, .figure(micros: 241_100_000, text: "+$241.10", tone: .gain))

        user.pnl1mMicros = -417_680_000
        let (loss, _, _) = try make([.profile(user)])
        await loss.load()
        XCTAssertEqual(loss.pnl1M, .figure(micros: -417_680_000, text: "−$417.68", tone: .loss))

        user.pnl1mMicros = 0
        let (flat, _, _) = try make([.profile(user)])
        await flat.load()
        XCTAssertEqual(flat.pnl1M, .figure(micros: 0, text: "$0.00", tone: .flat))
    }

    func testTheOneMonthFigureIsLoadingUntilTheReadReturnsAndFailedWhenItFails() async throws {
        let (loading, _, _) = try make([.profile(.sampleOwn)])
        XCTAssertEqual(loading.pnl1M, .loading)

        let (failing, _, _) = try make([.failure])
        await failing.load()
        XCTAssertEqual(failing.pnl1M, .failed)
        XCTAssertNil(failing.card)
    }

    func testAMissingOneMonthFigureIsFailedNeverAZero() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.pnl1mMicros = nil

        let (model, _, _) = try make([.profile(user)])
        await model.load()

        XCTAssertEqual(model.pnl1M, .failed)
        XCTAssertNotNil(model.card)
    }

    func testYourOneMonthPnLShowsWhenYouHidePerformanceFromOthers() async throws {
        var user = Components.Schemas.PublicProfile.sampleOwn
        user.returnPrivate = true
        user.pnl1mMicros = 241_100_000

        let (model, _, _) = try make([.profile(user)])
        await model.load()

        XCTAssertEqual(model.pnl1M, .figure(micros: 241_100_000, text: "+$241.10", tone: .gain))
    }

    func testAFailedReloadKeepsWhatWasShown() async throws {
        let (model, _, _) = try make([.profile(.sampleOwn), .failure])
        await model.load()

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.pnl1M, .figure(micros: 241_100_000, text: "+$241.10", tone: .gain))
    }

    func testTheReadAsksForTheViewersOwnUser() async throws {
        let (model, transport, _) = try make([.profile(.sampleOwn)])

        await model.load()

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertEqual(sent.map(\.path), ["/v1/users/\(Self.userID)"])
    }

    func testABalanceChangedHintAndAResyncReloadAndAPingDoesNot() async throws {
        var richer = Components.Schemas.PublicProfile.sampleOwn
        richer.pnl1mMicros = 500_000_000
        var richest = richer
        richest.pnl1mMicros = 900_000_000
        let (model, transport, hints) = try make([.profile(.sampleOwn), .profile(richer), .profile(richest)])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        await hints.waitForSubscribers(1)

        await hints.send(.changed(.user(Self.userID), what: "ping_echoed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let afterPing = await transport.sent.count
        XCTAssertEqual(afterPing, 1)

        await hints.send(.changed(.user(Self.userID), what: "balance_changed", id: "2"))
        let reloaded = await waitUntil { model.pnl1M == .figure(micros: 500_000_000, text: "+$500.00", tone: .gain) }
        XCTAssertTrue(reloaded)

        await hints.send(.resync)
        let resynced = await waitUntil { model.pnl1M == .figure(micros: 900_000_000, text: "+$900.00", tone: .gain) }
        XCTAssertTrue(resynced)
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    private enum Script {
        case profile(Components.Schemas.PublicProfile)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .profile(let user):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(user), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(_ script: [Script]) throws -> (OwnProfileModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let model = OwnProfileModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, viewerID: Self.userID)
        return (model, transport, hints)
    }

    private func waitUntil(_ predicate: @escaping () -> Bool) async -> Bool {
        for _ in 0..<50_000 {
            if predicate() { return true }
            await Task.yield()
        }
        return predicate()
    }
}
