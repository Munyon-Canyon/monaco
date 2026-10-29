import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class CabalMemberPageSampleTests: XCTestCase {
    func testACCapitalPageCarriesRecordsCrownAndTotals() {
        let page = Components.Schemas.CabalMemberPage.sampleACCapital
        XCTAssertEqual(page.items.count, 8)
        XCTAssertEqual(page.items.prefix(5).map(\.ownershipBps), [3500, 2000, 1200, 1000, 800])
        XCTAssertEqual(page.items.map(\.ownershipBps).reduce(0, +), 10000)
        XCTAssertEqual(page.items.filter(\.crown).map(\.displayName), ["Christelle"])
        XCTAssertEqual(page.items[0].month.right, 7)
        XCTAssertEqual(page.totals.admins, 2)
        XCTAssertEqual(page.totals.voters, 6)
        XCTAssertEqual(page.totals.members, 0)
        XCTAssertNil(page.nextCursor)
    }

    func testSampleFromMembersMapsRolesAndCountsAdmins() {
        let joined = Date(timeIntervalSince1970: 1_790_000_000)
        func member(_ id: String, _ role: String) -> Components.Schemas.CabalMember {
            .init(
                userId: id, handle: nil, displayName: id, photoUrl: nil, role: role, canVote: true, joinedAt: joined)
        }
        let page = Components.Schemas.CabalMemberPage.sample(members: [
            member("a", "creator"), member("b", "member"), member("c", "unknown"),
        ])
        XCTAssertEqual(page.items.map(\.ownershipBps), [10000, 0, 0])
        XCTAssertEqual(page.items[2].role, .member)
        XCTAssertEqual(page.totals.admins, 1)
        XCTAssertEqual(page.totals.voters, 2)
    }

    func testMemberRowInitReadsTheWireRow() {
        let row = Components.Schemas.CabalMemberPage.sampleACCapital.items[1]
        let wrapped = CabalMemberRow(row)
        XCTAssertEqual(wrapped.id, row.userId)
        XCTAssertEqual(wrapped.member.role, "admin")
        XCTAssertEqual(wrapped.member.displayName, "Christelle")
        XCTAssertTrue(wrapped.member.canVote)
    }
}
