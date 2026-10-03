import MonacoAPI
import MonacoCore
import XCTest

final class CabalRulesDiffTests: XCTestCase {
    private let creatorID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let current = CabalSettings(
        name: "QA pot",
        joinMode: "open",
        voters: .everyone,
        threshold: "unanimous",
        proposalExpirySeconds: 86_400
    )

    private func patch(_ edit: (inout CabalSettings) -> Void) -> Components.Schemas.UpdateCabalRequest {
        var edited = current
        edit(&edited)
        return CabalRulesDiff.patch(from: current, to: edited, creatorID: creatorID)
    }

    func testNoChangeGivesAnEmptyBody() {
        let body = patch { _ in }

        XCTAssertEqual(body, Components.Schemas.UpdateCabalRequest())
        XCTAssertTrue(body.isEmpty)
    }

    func testARenameSendsOnlyTheTrimmedName() {
        XCTAssertEqual(patch { $0.name = "  QA pot 2 " }, .init(name: "QA pot 2"))
    }

    func testWhitespaceAroundTheSameNameIsNoChange() {
        XCTAssertTrue(patch { $0.name = " QA pot  " }.isEmpty)
    }

    func testAJoinModeChangeSendsOnlyTheJoinMode() {
        XCTAssertEqual(patch { $0.joinMode = "request" }, .init(joinMode: "request"))
    }

    func testAThresholdChangeSendsOnlyTheThreshold() {
        XCTAssertEqual(patch { $0.threshold = "majority" }, .init(threshold: "majority"))
    }

    func testAnExpiryChangeSendsOnlyTheExpiry() {
        XCTAssertEqual(patch { $0.proposalExpirySeconds = 3600 }, .init(proposalExpirySeconds: 3600))
    }

    func testJustMeSendsAListNamingOnlyTheCreator() {
        XCTAssertEqual(patch { $0.voters = .justMe }, .init(voterMode: "list", voterIds: [creatorID]))
    }

    func testEveryoneSendsAllWithNoVoterIDs() {
        let justMe = CabalSettings(
            name: "QA pot", joinMode: "open", voters: .justMe, threshold: "unanimous", proposalExpirySeconds: 86_400)

        let body = CabalRulesDiff.patch(from: justMe, to: current, creatorID: creatorID)

        XCTAssertEqual(body, .init(voterMode: "all"))
    }

    func testSettingsReadAListVoterSetAsJustMe() throws {
        let raw =
            ##"{"id":"\##(creatorID)","name":"QA pot","picture_url":null,"status":"active","##
            + ##""rules":{"join_mode":"request","voter_mode":"list","threshold":"majority","##
            + ##""proposal_expiry_seconds":3600,"slippage_bps":100},"##
            + ##""creator":{"user_id":"\##(creatorID)","handle":"kai","display_name":"Kai","photo_url":null},"##
            + ##""member_count":1,"members":[],"me":{"role":"creator","can_vote":true},"my_access_request":null,"##
            + ##""invite_code":"ABCD2345","treasury_address":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"}"##
        let cabal = try JSONDecoder().decode(Components.Schemas.Cabal.self, from: Data(raw.utf8))

        XCTAssertEqual(
            CabalSettings(cabal),
            CabalSettings(
                name: "QA pot", joinMode: "request", voters: .justMe, threshold: "majority", proposalExpirySeconds: 3600
            )
        )
    }
}
