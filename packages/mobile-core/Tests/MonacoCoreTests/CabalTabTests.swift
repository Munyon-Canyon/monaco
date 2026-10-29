import MonacoCore
import XCTest

@MainActor
final class CabalTabTests: XCTestCase {
    func testASelectionStartsOnTheRoutesTabAndKeepsTheLastPick() async {
        let selection = CabalTabSelection(.members)
        XCTAssertEqual(selection.tab, .members)
        selection.tab = .voters
        XCTAssertEqual(selection.tab, .voters)
        XCTAssertEqual(CabalTabSelection().tab, .treasury)
    }

    func testTheTabsRunTreasuryRulesMembersVoters() async {
        XCTAssertEqual(CabalTab.allCases, [.treasury, .rules, .members, .voters])
        XCTAssertEqual(CabalTab.allCases.map(\.title), ["Treasury", "Rules", "Members", "Voters"])
    }

    func testOnlyMembersCarriesTheCount() async {
        XCTAssertEqual(CabalTab.members.title(memberCount: 8), "Members 8")
        XCTAssertEqual(CabalTab.members.count(memberCount: 8), 8)
        XCTAssertEqual(CabalTab.treasury.title(memberCount: 8), "Treasury")
        XCTAssertEqual(CabalTab.rules.title(memberCount: 8), "Rules")
        XCTAssertEqual(CabalTab.voters.title(memberCount: 8), "Voters")
        XCTAssertNil(CabalTab.voters.count(memberCount: 8))
    }

    func testMembersHasNoCountBeforeTheCabalLoads() async {
        XCTAssertEqual(CabalTab.members.title(memberCount: nil), "Members")
        XCTAssertNil(CabalTab.members.count(memberCount: nil))
    }

    func testTheDefaultTabIsTreasury() async {
        XCTAssertEqual(CabalTab.default, .treasury)
        XCTAssertEqual(CabalTab(rawValue: "members"), .members)
    }
}
