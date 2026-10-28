import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalStatsTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_792_662_780)
    private let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }()

    private func stats(
        _ fixture: Components.Schemas.ProposalDetail, asset: Components.Schemas.AssetDetail = .nvda
    ) -> ProposalStats {
        ProposalStats.make(
            detail: ProposalDetail(fixture), asset: ProposalAsset(asset), now: now, calendar: utc)
    }

    private func line(_ stats: ProposalStats) -> [String] {
        stats.cells.map { "\($0.label) \($0.value)" }
    }

    func testAnOpenBuyShowsPriceSharesTreasuryAndWhenItCloses() async throws {
        let result = stats(.openNVDA(now: now))
        XCTAssertEqual(line(result), ["Price $220.00", "Shares ≈ 1.82", "Of treasury 2%", "Closes 1:53 PM"])
        XCTAssertEqual(result.cells.map(\.hasInfo), [true, false, false, false])
    }

    func testAPassedBuyThatHasNotFilledShowsWhenItPassed() async throws {
        let result = stats(.passedNVDA(now: now))
        XCTAssertEqual(line(result), ["Price $220.00", "Shares ≈ 1.82", "Of treasury 2%", "Passed 9:41 AM"])
    }

    func testAFilledBuyShowsTheFillTheSharesTheLivePriceAndSince() async throws {
        let result = stats(.filledNVDA(now: now))
        XCTAssertEqual(line(result), ["Fill $219.40", "Shares 1.82", "Now $220.00", "Since +0.3%"])
        XCTAssertEqual(result.cells.map(\.tone), [.plain, .plain, .plain, .gain])
        XCTAssertEqual(result.cells.map(\.hasInfo), [false, false, false, false])
    }

    func testAFilledBuyInAnotherCabalReadsTheSameWay() async throws {
        XCTAssertEqual(
            line(stats(.filledMETA(now: now))), ["Fill $716.52", "Shares 0.42", "Now $724.40", "Since +1.1%"])
    }

    func testAFallShowsSinceInTheLossTone() async throws {
        var fixture = Components.Schemas.ProposalDetail.filledNVDA(now: now)
        fixture.stats.sinceBps = -125
        let result = stats(fixture)
        XCTAssertEqual(result.cells.last?.value, "\u{2212}1.3%")
        XCTAssertEqual(result.cells.last?.tone, .loss)
    }

    func testAFailedSellShowsExactSharesAndWhenItClosed() async throws {
        let result = stats(.failedTSLA(now: now))
        XCTAssertEqual(line(result), ["Price $412.55", "Shares 2.2", "Of treasury 4%", "Closed Oct 18"])
    }

    func testAWithdrawnAndVoidedProposalNameHowTheyEnded() async throws {
        for (status, label) in [(Components.Schemas.ProposalStatus.withdrawn, "Withdrawn"), (.voided, "Voided")] {
            var fixture = Components.Schemas.ProposalDetail.failedTSLA(now: now)
            fixture.status = status
            XCTAssertEqual(stats(fixture).cells.last?.label, label)
            XCTAssertEqual(stats(fixture).cells.last?.value, "Oct 18")
        }
    }

    func testAPreIpoProposalCallsItsPriceAnEstimate() async throws {
        let result = stats(.preIpoSpaceX(now: now), asset: .spaceX)
        XCTAssertEqual(result.cells.first?.label, "Estimate")
        XCTAssertEqual(result.cells.first?.hasInfo, true)
        XCTAssertEqual(result.cells.last?.value, "Fri 5:53 AM")
    }

    func testAClosingTimeBeyondAWeekReadsAsADate() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.closesAt = now.addingTimeInterval(9 * 86_400)
        XCTAssertEqual(stats(fixture).cells.last?.value, "Oct 31")
    }

    func testANullFigureShowsADashNeverZeroDollars() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.priceMicros = nil
        fixture.stats.sharesAtomics = nil
        fixture.stats.treasuryShareBps = nil
        XCTAssertEqual(line(stats(fixture)), ["Price —", "Shares —", "Of treasury —", "Closes 1:53 PM"])
    }

    func testANullClosedAtShowsADash() async throws {
        var fixture = Components.Schemas.ProposalDetail.failedTSLA(now: now)
        fixture.stats.closedAt = nil
        XCTAssertEqual(stats(fixture).cells.last?.value, "—")
        var passed = Components.Schemas.ProposalDetail.passedNVDA(now: now)
        passed.stats.closedAt = nil
        XCTAssertEqual(stats(passed).cells.last?.value, "—")
    }

    func testPercentsUseOneDecimalUnderTenAndNoneAbove() async throws {
        let cases: [(Int64, String)] = [(0, "0%"), (4, "<0.1%"), (250, "2.5%"), (995, "10%"), (1_240, "12%")]
        for (bps, expected) in cases {
            var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
            fixture.stats.treasuryShareBps = bps
            XCTAssertEqual(stats(fixture).cells[2].value, expected, "\(bps)")
        }
    }

    func testTheCrestAgeIsRelativeTodayAndADateAfterADay() async throws {
        XCTAssertEqual(ProposalStats.crestAge(since: now.addingTimeInterval(-7_200), now: now, calendar: utc), "2h")
        XCTAssertEqual(
            ProposalStats.crestAge(since: now.addingTimeInterval(-86_400 * 3), now: now, calendar: utc), "Oct 19")
    }
}
