import MonacoAPI
import MonacoCore
import XCTest

final class CabalRulesDiffTests: XCTestCase {
    private let creatorID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let current = CabalSettings(
        name: "QA pot",
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

    func testAThresholdChangeSendsOnlyTheThreshold() {
        XCTAssertEqual(patch { $0.threshold = "majority" }, .init(threshold: "majority"))
    }

    func testAnExpiryChangeSendsOnlyTheExpiry() {
        XCTAssertEqual(patch { $0.proposalExpirySeconds = 3600 }, .init(proposalExpirySeconds: 3600))
    }

    func testPickingVotersSendsTheModeAndTheCreatorFirstList() {
        let body = patch { $0.voters = .list(["z", "a"]) }

        XCTAssertEqual(body, .init(voterMode: "list", voterIds: [creatorID, "a", "z"]))
    }

    func testBackToEveryoneSendsOnlyTheMode() {
        let picked = CabalSettings(
            name: "QA pot", threshold: "unanimous", proposalExpirySeconds: 86_400,
            voters: .list([creatorID, "a"]))

        XCTAssertEqual(
            CabalRulesDiff.patch(from: picked, to: current, creatorID: creatorID), .init(voterMode: "all"))
    }

    func testTheCreatorAloneIsTheSameVoterSetWhetherOrNotItIsNamed() {
        let named = CabalSettings(
            name: "QA pot", threshold: "unanimous", proposalExpirySeconds: 86_400,
            voters: .list([creatorID]))
        var bare = named
        bare.voters = .list([])

        XCTAssertTrue(CabalRulesDiff.patch(from: named, to: bare, creatorID: creatorID).isEmpty)
    }

    func testVotersAndARuleShareOnePatch() {
        let body = patch {
            $0.threshold = "majority"
            $0.voters = .list(["a"])
        }

        XCTAssertEqual(body, .init(voterMode: "list", voterIds: [creatorID, "a"], threshold: "majority"))
    }

    func testSettingsReadTheRulesAndTheVoters() throws {
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
                name: "QA pot", threshold: "majority", proposalExpirySeconds: 3600,
                voters: .list([])
            )
        )
    }
}
