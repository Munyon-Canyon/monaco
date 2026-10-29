import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class PassedMomentStoreTests: XCTestCase {
    private var clock = Date(timeIntervalSince1970: 1_792_662_780)
    private let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }()

    private func makeDefaults() throws -> UserDefaults {
        let suite = "PassedMomentStoreTests.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        addTeardownBlock { UserDefaults(suiteName: suite)?.removePersistentDomain(forName: suite) }
        return defaults
    }

    private func makeStore(_ defaults: UserDefaults, account: String = "user-1") -> PassedMomentStore {
        PassedMomentStore(defaults: defaults, accountID: account, now: { [unowned self] in clock })
    }

    private func detail(
        status: Components.Schemas.ProposalStatus = .executed, ballot: Components.Schemas.BallotChoice? = .yes,
        id: String = "proposal-1", filledAgo: TimeInterval = 720, fills: Bool = true
    ) -> ProposalDetail {
        var value = Components.Schemas.ProposalDetail.filledNVDA(now: clock)
        value.id = id
        value.status = status
        value.myBallot = ballot.flatMap { .init(rawValue: $0.rawValue) }
        if fills {
            value.swap?.filledAt = clock.addingTimeInterval(-filledAgo)
        } else {
            value.swap?.filledAt = nil
        }
        return ProposalDetail(value)
    }

    func testAVoterSeesAFilledPassOnce() async throws {
        let store = makeStore(try makeDefaults())
        let fixture = detail()
        XCTAssertTrue(store.shouldShow(detail: fixture))
        store.markSeen(id: fixture.id, filledAt: clock.addingTimeInterval(-720))
        XCTAssertFalse(store.shouldShow(detail: fixture))
    }

    func testARestartKeepsTheSeenSet() async throws {
        let defaults = try makeDefaults()
        let fixture = detail()
        makeStore(defaults).markSeen(id: fixture.id, filledAt: clock.addingTimeInterval(-720))
        XCTAssertFalse(makeStore(defaults).shouldShow(detail: fixture))
    }

    func testPassedAndExecutedQualifyAndNothingElseDoes() async throws {
        let store = makeStore(try makeDefaults())
        XCTAssertTrue(store.shouldShow(detail: detail(status: .passed)))
        XCTAssertTrue(store.shouldShow(detail: detail(status: .executed)))
        let others: [Components.Schemas.ProposalStatus] = [
            .open, .failed, .expired, .withdrawn, .voided, .executionBlocked,
        ]
        for status in others {
            XCTAssertFalse(store.shouldShow(detail: detail(status: status)), "\(status)")
        }
    }

    func testANonVoterNeverSeesIt() async throws {
        XCTAssertFalse(makeStore(try makeDefaults()).shouldShow(detail: detail(ballot: nil)))
    }

    func testAYesAndANoBothQualify() async throws {
        let store = makeStore(try makeDefaults())
        XCTAssertTrue(store.shouldShow(detail: detail(ballot: .yes)))
        XCTAssertTrue(store.shouldShow(detail: detail(ballot: .no)))
    }

    func testAPassWhoseSwapHasNotFilledDoesNotShow() async throws {
        let store = makeStore(try makeDefaults())
        XCTAssertFalse(store.shouldShow(detail: detail(fills: false)))
        var pending = Components.Schemas.ProposalDetail.filledNVDA(now: clock)
        pending.myBallot = .init(rawValue: "yes")
        pending.swap?.status = .submitted
        XCTAssertFalse(store.shouldShow(detail: ProposalDetail(pending)))
        var swapless = pending
        swapless.swap = nil
        XCTAssertFalse(store.shouldShow(detail: ProposalDetail(swapless)))
    }

    func testTheWindowIsSevenDays() async throws {
        let store = makeStore(try makeDefaults())
        XCTAssertEqual(PassedMomentStore.window, 7 * 86_400)
        XCTAssertTrue(store.shouldShow(detail: detail(filledAgo: PassedMomentStore.window - 60)))
        XCTAssertFalse(store.shouldShow(detail: detail(filledAgo: PassedMomentStore.window + 60)))
    }

    func testTheMomentAppearsOnceTheFillArrivesAndNotBefore() async throws {
        let store = makeStore(try makeDefaults())
        XCTAssertFalse(store.shouldShow(detail: detail(fills: false)))
        XCTAssertTrue(store.shouldShow(detail: detail(fills: true)))
    }

    func testFiveHundredSeenProposalsNeverBringTheFirstBack() async throws {
        let store = makeStore(try makeDefaults())
        let firstFill = clock.addingTimeInterval(-720)
        store.markSeen(id: "first", filledAt: firstFill)
        for index in 0..<500 {
            store.markSeen(id: "proposal-\(index)", filledAt: clock.addingTimeInterval(-TimeInterval(index)))
        }
        XCTAssertFalse(store.shouldShow(detail: detail(id: "first")))
        XCTAssertFalse(store.shouldShow(detail: detail(id: "proposal-499", filledAgo: 499)))
    }

    func testAnEntryIsPrunedOnlyOnceItsFillIsFourteenDaysOld() async throws {
        let defaults = try makeDefaults()
        let store = makeStore(defaults)
        let fill = clock.addingTimeInterval(-720)
        store.markSeen(id: "old", filledAt: fill)

        clock = fill.addingTimeInterval(PassedMomentStore.retention - 60)
        store.markSeen(id: "other", filledAt: clock)
        let stored = defaults.dictionary(forKey: "passedMomentsSeen.user-1") as? [String: Double]
        XCTAssertNotNil(stored?["old"])

        clock = fill.addingTimeInterval(PassedMomentStore.retention + 60)
        store.markSeen(id: "later", filledAt: clock)
        let pruned = defaults.dictionary(forKey: "passedMomentsSeen.user-1") as? [String: Double]
        XCTAssertNil(pruned?["old"])
        XCTAssertNotNil(pruned?["later"])
    }

    func testPruningNeverLetsAnOldPassShowAgain() async throws {
        let store = makeStore(try makeDefaults())
        let fixture = detail()
        store.markSeen(id: fixture.id, filledAt: clock.addingTimeInterval(-720))
        clock = clock.addingTimeInterval(PassedMomentStore.retention + 3_600)
        store.markSeen(id: "later", filledAt: clock)
        let stale = detail(filledAgo: PassedMomentStore.retention + 3_600 + 720)
        XCTAssertFalse(store.shouldShow(detail: stale))
    }

    func testTwoAccountsKeepSeparateSeenSets() async throws {
        let defaults = try makeDefaults()
        let book = PassedMomentBook(defaults: defaults, now: { [unowned self] in clock })
        let fixture = detail()
        book.store(for: "user-a").markSeen(id: fixture.id, filledAt: clock.addingTimeInterval(-720))
        XCTAssertFalse(book.store(for: "user-a").shouldShow(detail: fixture))
        XCTAssertTrue(book.store(for: "user-b").shouldShow(detail: fixture))
    }

    func testTheBookCachesOneStorePerAccount() async throws {
        let book = PassedMomentBook(defaults: try makeDefaults())
        XCTAssertTrue(book.store(for: "user-a") === book.store(for: "user-a"))
        XCTAssertFalse(book.store(for: "user-a") === book.store(for: "user-b"))
    }

    func testTheSheetCopyMatchesTheFrame() async throws {
        let fixture = detail()
        let summary = fixture.summary
        let swap = try XCTUnwrap(summary.swap)
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTitle(
                kind: summary.kind, symbol: summary.symbol, assetKind: .stock, amountMicros: summary.usdcMicros),
            "Buy NVDA · $400")
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTitle(
                kind: "sell", symbol: "TSLAx", assetKind: .stock, amountMicros: 900_000_000),
            "Sell TSLA · $900")
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTally(
                tally: ProposalTally(yes: 5, no: 1, voters: 8, needed: 5), closedAt: clock.addingTimeInterval(-720),
                now: clock, calendar: utc),
            "✓ 5 · ✕ 1 · closed 9:41 AM")
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTally(
                tally: ProposalTally(yes: 5, no: 1, voters: 8, needed: 5),
                closedAt: clock.addingTimeInterval(-3 * 86_400), now: clock, calendar: utc),
            "✓ 5 · ✕ 1 · closed Oct 19")
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTrade(swap, symbol: "NVDAx", assetKind: .stock, isSell: false),
            "Bought 1.82 NVDA at $219.40.")
        XCTAssertEqual(
            ProposalResultCopy.passedSheetTrade(swap, symbol: "TSLAx", assetKind: .stock, isSell: true),
            "Sold 1.82 TSLA at $219.40.")
        XCTAssertEqual(
            ProposalResultCopy.calledItLine(proposerName: "Mia", isViewer: false, voted: 6, voters: 8),
            "Mia called it · 6 of 8 voted")
        XCTAssertEqual(
            ProposalResultCopy.calledItLine(proposerName: "Mia", isViewer: true, voted: 6, voters: 8),
            "You called it · 6 of 8 voted")
    }
}
