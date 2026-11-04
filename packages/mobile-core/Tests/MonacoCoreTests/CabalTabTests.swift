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

    func testTheTabsRunChatTreasuryRulesMembersVoters() async {
        XCTAssertEqual(CabalTab.allCases, [.chat, .treasury, .rules, .members, .voters])
        XCTAssertEqual(CabalTab.allCases.map(\.title), ["Chat", "Treasury", "Rules", "Members", "Voters"])
    }

    func testChatLeadsTheTabsUnlessTheCabalIsReadOnly() async {
        XCTAssertTrue(CabalTab.hostsChat)
        XCTAssertEqual(CabalTab.shown, [.chat, .treasury, .rules, .members, .voters])
        XCTAssertEqual(CabalTab.shown(isReadOnly: false), CabalTab.shown)
        XCTAssertEqual(CabalTab.shown(isReadOnly: true), [.treasury, .rules, .members, .voters])
        XCTAssertEqual(CabalTab(rawValue: "chat"), .chat)
    }

    func testChatCarriesNoCount() async {
        XCTAssertNil(CabalTab.chat.count(memberCount: 8))
        XCTAssertEqual(CabalTab.chat.title(memberCount: 8), "Chat")
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
