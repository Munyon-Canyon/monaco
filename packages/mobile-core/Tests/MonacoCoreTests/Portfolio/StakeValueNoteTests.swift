import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class StakeValueNoteTests: XCTestCase {
    private var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar
    }

    private let now = Date(timeIntervalSince1970: 1_791_590_400 + 15 * 3_600)

    private func stake(
        value: Int64, arriving: Int64? = nil, stale: Bool? = nil, valuedAt: Date? = nil, returnBps: Int64? = 250
    ) -> Components.Schemas.PortfolioCabal {
        var cabal = Components.Schemas.PortfolioCabal(
            cabal: Components.Schemas.CabalRef(id: "cabal-1", name: "Alpha", pictureUrl: nil), valueMicros: value,
            shareUnits: 10, netContributedMicros: value, pnlMicros: 0, returnBps: returnBps, sliceBps: 0,
            ownershipBps: 5_000, role: .member, canVote: true, potValueMicros: 0, memberCount: 3, openVoteCount: 0)
        cabal.arrivingMicros = arriving
        cabal.stale = stale
        cabal.valuedAt = valuedAt
        return cabal
    }

    private func summary(_ cabal: Components.Schemas.PortfolioCabal) -> PortfolioSummary {
        PortfolioSummary(
            Components.Schemas.MyPortfolio(
                cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
                pricesAsOf: nil, cabals: [cabal]))
    }

    func testAFreshValuedStakeHasNoNote() {
        XCTAssertNil(StakeValueNote.make(valueMicros: 5_000_000, arrivingMicros: 0, isStale: false, valuedAt: nil))
        XCTAssertNil(
            StakeValueNote.make(valueMicros: 5_000_000, arrivingMicros: 1_000_000, isStale: false, valuedAt: nil))
    }

    func testAStakeNoRunHasValuedIsSettling() {
        XCTAssertEqual(
            StakeValueNote.make(valueMicros: 0, arrivingMicros: 100_000_000, isStale: false, valuedAt: nil), .settling)
        XCTAssertEqual(
            StakeValueNote.make(valueMicros: 0, arrivingMicros: 100_000_000, isStale: true, valuedAt: nil), .settling)
        XCTAssertNil(StakeValueNote.make(valueMicros: 0, arrivingMicros: -100_000_000, isStale: false, valuedAt: nil))
        XCTAssertNil(StakeValueNote.make(valueMicros: 0, arrivingMicros: 0, isStale: false, valuedAt: nil))
    }

    func testAStaleStakeSaysWhenItWasValued() {
        let sameDay = now.addingTimeInterval(-2 * 3_600)
        let earlier = now.addingTimeInterval(-3 * 86_400)
        XCTAssertEqual(
            StakeValueNote.make(valueMicros: 5_000_000, arrivingMicros: 0, isStale: true, valuedAt: sameDay),
            .valueAsOf(sameDay))
        XCTAssertEqual(StakeValueNote.valueAsOf(sameDay).text(now: now, calendar: calendar), "Value as of 1:00 PM")
        XCTAssertEqual(StakeValueNote.valueAsOf(earlier).text(now: now, calendar: calendar), "Value as of Oct 7")
        XCTAssertEqual(StakeValueNote.valueAsOf(nil).text(now: now, calendar: calendar), "Value from an earlier run")
        XCTAssertEqual(StakeValueNote.settling.text(now: now, calendar: calendar), "Settling")
    }

    func testTheRowsCarryTheWireFlags() {
        let valued = now.addingTimeInterval(-3 * 86_400)
        let row = summary(stake(value: 5_000_000, stale: true, valuedAt: valued)).rows[0]
        XCTAssertTrue(row.isStale)
        XCTAssertEqual(row.valuedAt, valued)
        let plain = summary(stake(value: 5_000_000)).rows[0]
        XCTAssertFalse(plain.isStale)
        XCTAssertNil(plain.valuedAt)
    }

    func testHomeShowsTheNoteAndHidesTheReturnOfASettlingStake() {
        let stale = HomeCabalRow(summary(stake(value: 5_000_000, stale: true, valuedAt: now)).rows[0])
        XCTAssertEqual(stale.note, .valueAsOf(now))
        XCTAssertEqual(stale.returnBps, 250)
        XCTAssertEqual(stale.noteText(now: now, calendar: calendar), "Value as of 3:00 PM")

        let settling = HomeCabalRow(summary(stake(value: 0, arriving: 100_000_000, returnBps: nil)).rows[0])
        XCTAssertEqual(settling.note, .settling)
        XCTAssertEqual(settling.valueMicros, 100_000_000)
        XCTAssertNil(settling.returnBps)
        XCTAssertEqual(settling.noteText(now: now, calendar: calendar), "Settling")

        let fresh = HomeCabalRow(summary(stake(value: 5_000_000)).rows[0])
        XCTAssertNil(fresh.note)
        XCTAssertNil(fresh.noteText(now: now, calendar: calendar))
    }

    func testProfileCarriesTheNoteAndADashForAMissingReturn() {
        let cabal = Components.Schemas.MyCabal(
            id: "cabal-1", name: "Alpha", pictureUrl: nil,
            status: "active", role: "member", canVote: true, memberCount: 3,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0, unreadCount: 0)
        func member(_ stake: Components.Schemas.PortfolioCabal) -> ProfileCabalRow.Member? {
            let portfolio = Components.Schemas.MyPortfolio(
                cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
                pricesAsOf: nil, cabals: [stake])
            let rows = ProfileCabalRows.make(cabals: [cabal], portfolio: portfolio, invites: [])
            guard case .member(let member) = rows[0] else { return nil }
            return member
        }

        let stale = member(stake(value: 5_000_000, stale: true, valuedAt: now))
        XCTAssertEqual(stale?.note, .valueAsOf(now))
        XCTAssertEqual(stale?.value, "$5.00")

        let unknown = member(stake(value: 5_000_000, returnBps: nil))
        XCTAssertEqual(unknown?.returnText, "—")
        XCTAssertEqual(unknown?.returnTone, .flat)
        XCTAssertNil(unknown?.note)

        var unvalued = stake(value: 0, arriving: 100_000_000, returnBps: nil)
        unvalued.shareUnits = 0
        let fresh = member(unvalued)
        XCTAssertEqual(fresh?.note, .settling)
        XCTAssertEqual(fresh?.value, "$100.00")

        var topped = stake(value: 0, arriving: 100_000_000, returnBps: nil)
        topped.shareUnits = 10
        XCTAssertEqual(member(topped)?.note, .settling)
    }
}
