import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class CabalRulesRowsTests: XCTestCase {
    private let calendar: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar
    }()
    private let locale = Locale(identifier: "en_US")

    private func rows(_ cabal: Components.Schemas.Cabal, now: Date = .now) -> CabalRulesRows {
        CabalRulesRows(cabal, now: now, calendar: calendar, locale: locale)
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
        XCTAssertEqual(value(result, .voteWindow), "24 h")
        XCTAssertEqual(value(result, .maxPerTrade), "20% of cash")
        XCTAssertEqual(value(result, .whoCanPropose), "Voters")
        XCTAssertEqual(value(result, .cashOut), "Anytime, your share")
        XCTAssertEqual(value(result, .showSides), "After a vote closes \u{00B7} off")
        XCTAssertFalse(result.showSides)
    }

    func testUnanimousNeedsEveryVoter() async {
        XCTAssertEqual(value(rows(cabal(threshold: "unanimous")), .votesToPass), "8 of 8")
    }

    func testTheSupportedVoteWindows() async {
        var cabal = cabal()
        cabal.rules.proposalExpirySeconds = 3600
        XCTAssertEqual(value(rows(cabal), .voteWindow), "1 h")
        cabal.rules.proposalExpirySeconds = 604_800
        XCTAssertEqual(value(rows(cabal), .voteWindow), "7 d")
        cabal.rules.proposalExpirySeconds = 7200
        XCTAssertEqual(value(rows(cabal), .voteWindow), "2 hours")
    }

    func testMaxPerTradeWithoutACapIsNoLimit() async {
        var cabal = cabal()
        cabal.rules.maxTradeBps = nil
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "No limit")
    }

    func testMaxPerTradeKeepsHundredths() async {
        var cabal = cabal()
        cabal.rules.maxTradeBps = 1250
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "12.50% of cash")
        cabal.rules.maxTradeBps = 1205
        XCTAssertEqual(value(rows(cabal), .maxPerTrade), "12.05% of cash")
    }

    func testSidesOnWithoutADateReadsOn() async {
        var cabal = cabal()
        cabal.rules.showSides = true
        let result = rows(cabal)
        XCTAssertEqual(value(result, .showSides), "After a vote closes \u{00B7} on")
        XCTAssertTrue(result.showSides)
    }

    func testSidesOnThisYearReadsMonthAndDay() async throws {
        let now = Date()
        let year = calendar.component(.year, from: now)
        let since = try XCTUnwrap(calendar.date(from: DateComponents(year: year, month: 10, day: 20, hour: 12)))
        var cabal = cabal()
        cabal.rules.showSides = true
        cabal.rules.showSidesSince = since

        XCTAssertEqual(value(rows(cabal, now: now), .showSides), "After a vote closes \u{00B7} on since Oct 20")
    }

    func testSidesOnInAnEarlierYearAddsTheYear() async throws {
        let now = Date()
        let year = calendar.component(.year, from: now) - 1
        let since = try XCTUnwrap(calendar.date(from: DateComponents(year: year, month: 10, day: 20, hour: 12)))
        var cabal = cabal()
        cabal.rules.showSides = true
        cabal.rules.showSidesSince = since

        XCTAssertEqual(
            value(rows(cabal, now: now), .showSides), "After a vote closes \u{00B7} on since Oct 20, \(year)")
    }

    func testTheFooterNamesTheAdmins() async {
        var cabal = cabal()
        let prefix = CabalRulesRows.footerPrefix
        cabal.admins = [person("Andres")]
        XCTAssertEqual(rows(cabal).footer, prefix + "the admin is Andres.")
        cabal.admins = [person("Andres"), person("Christelle")]
        XCTAssertEqual(rows(cabal).footer, prefix + "admins are Andres and Christelle.")
        cabal.admins = [person("Andres"), person("Christelle"), person("Logan")]
        XCTAssertEqual(rows(cabal).footer, prefix + "admins are Andres, Christelle and Logan.")
    }

    func testTheFooterFallsBackToTheCreatorAndHandles() async {
        var cabal = cabal()
        cabal.admins = nil
        XCTAssertTrue(rows(cabal).footer.hasSuffix("the admin is Kai."))
        cabal.admins = [person("", handle: "andres")]
        XCTAssertTrue(rows(cabal).footer.hasSuffix("the admin is andres."))
        cabal.admins = [person("", handle: nil)]
        XCTAssertTrue(rows(cabal).footer.hasSuffix("the admin is someone."))
    }

    func testTheFooterSaysTheWholePolicy() async {
        var cabal = cabal()
        cabal.admins = [person("Andres"), person("Christelle")]
        XCTAssertEqual(
            rows(cabal).footer,
            "Counts are live for every member. With this on, names by side show after a vote closes. "
                + "Rules change by proposal; admins are Andres and Christelle.")
    }
}
