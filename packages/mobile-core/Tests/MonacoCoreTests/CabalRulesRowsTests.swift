import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class CabalRulesRowsTests: XCTestCase {
    private func rows(_ cabal: Components.Schemas.Cabal) -> CabalRulesRows {
        CabalRulesRows(cabal)
    }

    private func value(_ rows: CabalRulesRows, _ kind: CabalRulesRows.Kind) -> String {
        rows.rows.first { $0.kind == kind }?.value ?? ""
    }

    private func cabal(voters: Int32 = 8, threshold: String = "majority") -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.voterCount = voters
        cabal.rules.threshold = threshold
        cabal.rules.maxTradeBps = 2000
        return cabal
    }

    private func person(_ name: String, handle: String? = nil) -> Components.Schemas.CabalPerson {
        .init(userId: "id-\(name)", handle: handle, displayName: name, photoUrl: nil)
    }

    func testTheSixRowsReadTheCabalsRules() async {
        let result = rows(cabal())

        XCTAssertEqual(result.rows.map(\.kind), CabalRulesRows.Kind.allCases)
        XCTAssertEqual(
            result.rows.map(\.title),
            ["Votes to pass", "Vote window", "Max per trade", "Who can propose", "Cash out", "Show who voted what"])
        XCTAssertEqual(value(result, .votesToPass), "5 of 8")
        XCTAssertEqual(value(result, .voteWindow), "24 hours")
        XCTAssertEqual(value(result, .maxPerTrade), "20% of the cash")
        XCTAssertEqual(value(result, .whoCanPropose), "Voters")
        XCTAssertEqual(value(result, .cashOut), "Any time")
        XCTAssertEqual(value(result, .showSides), "")
        XCTAssertFalse(result.showSides)
    }

    func testUnanimousNeedsEveryVoter() async {
        XCTAssertEqual(value(rows(cabal(threshold: "unanimous")), .votesToPass), "8 of 8")
    }

    func testTheSupportedVoteWindows() async {
        var cabal = cabal()
        cabal.rules.proposalExpirySeconds = 3600
        XCTAssertEqual(value(rows(cabal), .voteWindow), "1 hour")
        cabal.rules.proposalExpirySeconds = 86_400
        XCTAssertEqual(value(rows(cabal), .voteWindow), "24 hours")
        cabal.rules.proposalExpirySeconds = 604_800
        XCTAssertEqual(value(rows(cabal), .voteWindow), "7 days")
        cabal.rules.proposalExpirySeconds = 7200
        XCTAssertEqual(value(rows(cabal), .voteWindow), "2 hours")
        cabal.rules.proposalExpirySeconds = 21_600
        XCTAssertEqual(value(rows(cabal), .voteWindow), "6 hours")
        cabal.rules.proposalExpirySeconds = 5_400
        XCTAssertEqual(value(rows(cabal), .voteWindow), "1 hour")
    }

    func testMaxPerTradeWithoutACapIsNoLimit() async {
        var cabal = cabal()
        cabal.rules.maxTradeBps = nil
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "No limit")
    }

    func testMaxPerTradeKeepsHundredths() async {
        var cabal = cabal()
        cabal.rules.maxTradeBps = 1250
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "12.50% of the cash")
        cabal.rules.maxTradeBps = 1205
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "12.05% of the cash")
    }

    func testTheFooterIsOneSentencePerIdeaAndNamesNoAdmin() async {
        var cabal = cabal()
        cabal.admins = [person("Andres"), person("Christelle")]
        let expected =
            "Everyone sees the count as it happens. With this on, everyone also sees who voted which way "
            + "once a vote closes. Rules change by proposal."
        XCTAssertEqual(rows(cabal).footer, expected)
        XCTAssertEqual(CabalRulesRows.footer, expected)
        XCTAssertFalse(rows(cabal).footer.contains("Andres"))
        XCTAssertFalse(rows(cabal).footer.contains("admin"))
    }

    func testTheShowSidesRowCarriesNoValueText() async {
        var cabal = cabal()
        cabal.rules.showSides = true
        let result = rows(cabal)
        XCTAssertEqual(value(result, .showSides), "")
        XCTAssertTrue(result.showSides)
    }
}
