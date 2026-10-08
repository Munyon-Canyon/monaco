import Foundation
import MonacoAnalytics
import XCTest

final class AnalyticsStepTests: XCTestCase {
    private static let docFlowLabels: [String: AnalyticsFlow] = [
        "Onboarding": .onboarding,
        "Crypto deposit": .cryptoDeposit,
        "Card deposit": .cardDeposit,
        "Join cabal": .joinCabal,
        "Propose": .propose,
        "Vote": .vote,
        "Cash out": .cashOut,
        "Withdraw": .withdraw,
        "Feed": .feed,
        "Social": .social,
        "Chat": .chat,
        "Referral": .referral,
    ]

    private static let serverOnlyLabels: Set<String> = ["Trade outcome"]

    private func docTable() throws -> [String: [String]] {
        let url = RepoTree.root.appendingPathComponent("docs/architecture/analytics-admin.md")
        let text = try String(contentsOf: url, encoding: .utf8)
        let marker = "**Flows to instrument.**"
        let section = try XCTUnwrap(text.components(separatedBy: marker).last)
        var rows: [String: [String]] = [:]
        for line in section.split(separator: "\n") {
            if line.hasPrefix("## ") { break }
            guard line.hasPrefix("|") else { continue }
            let cells = line.split(separator: "|", omittingEmptySubsequences: false).dropFirst().dropLast()
                .map { $0.trimmingCharacters(in: .whitespaces) }
            guard cells.count == 2, cells[0] != "Flow", !cells[0].hasPrefix("---") else { continue }
            rows[cells[0]] = clientSteps(in: cells[1])
        }
        return rows
    }

    private func clientSteps(in cell: String) -> [String] {
        cell.replacingOccurrences(of: ";", with: "→").components(separatedBy: "→")
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.hasPrefix("(server)") }
            .flatMap { $0.components(separatedBy: "/") }
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
    }

    private func expectedName(flow: AnalyticsFlow, step: String) -> String {
        flow == .referral ? step : "\(flow.rawValue)_\(step)"
    }

    func testStepNamesMatchTheDocFlowTable() throws {
        let table = try docTable()
        var expected: Set<String> = []
        for (label, steps) in table where !Self.serverOnlyLabels.contains(label) {
            let flow = try XCTUnwrap(Self.docFlowLabels[label], "doc row \(label) has no AnalyticsFlow")
            expected.formUnion(steps.map { expectedName(flow: flow, step: $0) })
        }
        let actual = Set(AnalyticsStep.all.map(\.name))
        XCTAssertEqual(actual.subtracting(expected).sorted(), [], "steps missing from the doc table")
        XCTAssertEqual(expected.subtracting(actual).sorted(), [], "doc steps missing from AnalyticsStep")
        XCTAssertEqual(actual.count, AnalyticsStep.all.count, "two steps share an event name")
    }

    func testEveryDocRowIsKnown() throws {
        let labels = Set(try docTable().keys)
        XCTAssertEqual(labels, Set(Self.docFlowLabels.keys).union(Self.serverOnlyLabels))
    }

    func testEveryFlowHasSteps() {
        let flows = Set(AnalyticsStep.all.map(\.flow))
        XCTAssertEqual(flows, Set(AnalyticsFlow.allCases))
    }

    func testNamedSteps() {
        XCTAssertEqual(AnalyticsStep.onboarding(.loginStarted).name, "onboarding_login_started")
        XCTAssertEqual(AnalyticsStep.cryptoDeposit(.depositOpened).name, "crypto_deposit_deposit_opened")
        XCTAssertEqual(AnalyticsStep.cryptoDeposit(.addressCopied).name, "crypto_deposit_address_copied")
        XCTAssertEqual(AnalyticsStep.joinCabal(.fundSheetOpened).name, "join_cabal_fund_sheet_opened")
        XCTAssertEqual(AnalyticsStep.cashOut(.confirmed).name, "cash_out_confirmed")
        XCTAssertEqual(AnalyticsStep.referral(.invitePasteShown).name, "invite_paste_shown")
        XCTAssertEqual(AnalyticsStep.referral(.invitePasted).name, "invite_pasted")
        XCTAssertEqual(AnalyticsStep.referral(.inviteSkipped).name, "invite_skipped")
    }

    func testNamesAreSnakeCaseAndAvoidBannedWords() {
        let banned = ["xstock", "group", "mint"]
        for step in AnalyticsStep.all {
            XCTAssertNotNil(step.name.wholeMatch(of: /[a-z]+(_[a-z]+)*/), step.name)
            for word in banned {
                XCTAssertFalse(step.name.contains(word), "\(step.name) contains \(word)")
            }
        }
    }
}
