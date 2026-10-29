import Foundation
import MonacoCore
import XCTest

final class PushActionTests: XCTestCase {
    private let cabal = "0192e8a1-0002-7000-8000-000000000002"
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"

    private var payload: [String: Any] { ["kind": "vote_reminder", "proposal_id": proposal, "cabal_id": cabal] }

    func testYesAndNoOpenTheHoldSheetWithThePick() {
        XCTAssertEqual(
            PushAction.route(actionIdentifier: "VOTE_YES", payload: payload),
            .voteHold(proposalID: proposal, cabalID: cabal, pick: .yes))
        XCTAssertEqual(
            PushAction.route(actionIdentifier: "VOTE_NO", payload: payload),
            .voteHold(proposalID: proposal, cabalID: cabal, pick: .no))
    }

    func testReadTheThesisOpensTheProposal() {
        XCTAssertEqual(
            PushAction.route(actionIdentifier: "READ_THESIS", payload: payload),
            .proposal(proposalID: proposal, cabalID: cabal))
    }

    func testDefaultAndUnknownActionsRouteLikeATap() {
        for id in ["com.apple.UNNotificationDefaultActionIdentifier", "SOMETHING_ELSE"] {
            XCTAssertEqual(
                PushAction.route(actionIdentifier: id, payload: payload),
                PushRoute.parse(payload))
        }
    }

    func testMissingOrInvalidIDsFallBackToHome() {
        for bad: [String: Any] in [
            [:], ["proposal_id": "nope", "cabal_id": cabal], ["proposal_id": proposal],
            ["proposal_id": proposal, "cabal_id": 7],
        ] {
            for id in ["VOTE_YES", "VOTE_NO", "READ_THESIS"] {
                XCTAssertEqual(PushAction.route(actionIdentifier: id, payload: bad), PushRoute.parse(bad))
            }
        }
        XCTAssertEqual(PushAction.route(actionIdentifier: "VOTE_YES", payload: [:]), .home)
    }

    func testAnalyticsNames() {
        XCTAssertEqual(PushAction.analyticsName(actionIdentifier: "VOTE_YES"), "vote_yes")
        XCTAssertEqual(PushAction.analyticsName(actionIdentifier: "VOTE_NO"), "vote_no")
        XCTAssertEqual(PushAction.analyticsName(actionIdentifier: "READ_THESIS"), "read_thesis")
        XCTAssertEqual(PushAction.analyticsName(actionIdentifier: "x"), "default")
    }

    func testCategoriesCarryTheDocumentedActions() {
        let byID = Dictionary(uniqueKeysWithValues: PushCategory.all.map { ($0.id, $0) })
        XCTAssertEqual(byID["VOTE"]?.actions.map(\.id), ["VOTE_YES", "VOTE_NO", "READ_THESIS"])
        XCTAssertEqual(byID["VOTE_DECIDING"]?.actions.map(\.id), ["VOTE_YES", "VOTE_NO"])
        XCTAssertEqual(byID["LEDGER"]?.actions, [])
        for category in ["VOTE", "VOTE_DECIDING"] {
            for action in byID[category]?.actions ?? [] where action.id != "READ_THESIS" {
                XCTAssertTrue(action.foreground)
                XCTAssertTrue(action.authenticationRequired)
            }
        }
        XCTAssertFalse(byID["VOTE"]?.actions.last?.authenticationRequired ?? true)
    }

    func testPushActionNeverCallsTheVoteAPI() throws {
        let source = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/MonacoCore/Push/PushAction.swift")
        let text = try String(contentsOf: source, encoding: .utf8)
        XCTAssertFalse(text.contains("votes"))
        XCTAssertFalse(text.lowercased().contains("castvote"))
    }
}
