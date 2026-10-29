import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FirstFundModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let zoeID = "01890a5d-ac96-774b-bcce-b302099a8099"
    private let otherCabalID = "01890a5d-ac96-774b-bcce-b302099a8061"

    private var potPath: String { "/v1/cabals/\(cabalID)/pot" }
    private var cabalPath: String { "/v1/cabals/\(cabalID)" }
    private let votesPath = "/v1/me/pending-votes"
    private var membersPath: String { "/v1/cabals/\(cabalID)/members?limit=12&order=role_share" }

    private func pot(sliceBps: Int) -> String {
        #"{"cabal_id":"\#(cabalID)","pot_value_micros":20250000000,"cash_micros":20250000000,"#
            + #""cash_weight_bps":10000,"pnl_micros":0,"return_bps":null,"prices_as_of":"2026-10-29T12:00:00Z","#
            + #""holdings":[],"me":{"share_units":250000000,"value_micros":250000000,"slice_bps":\#(sliceBps),"#
            + #""net_contributed_micros":250000000,"pnl_micros":0,"first_funded_at":"2026-10-29T12:00:00Z"}}"#
    }

    private func vote(cabal: String, id: String, closes: String) -> String {
        #"{"proposal_id":"\#(id)","cabal_id":"\#(cabal)","cabal_name":"AC Capital","kind":"buy","symbol":"NVDAx","#
            + #""usdc_micros":400000000,"token_amount":null,"asset_name":"NVIDIA","asset_logo_url":null,"#
            + #""proposer":{"user_id":"\#(zoeID)","handle":"mia","display_name":"Mia","photo_url":null},"#
            + #""tally":{"yes":4,"no":1,"voters":9,"needed":5},"voted_count":5,"reply_count":6,"#
            + #""created_at":"2026-10-29T09:00:00Z","expires_at":"\#(closes)"}"#
    }

    private var anotherCabalsVote: String {
        vote(cabal: otherCabalID, id: "01890a5d-ac96-774b-bcce-b302099a8071", closes: "2026-10-29T13:00:00Z")
    }

    private func encode<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private func routes(
        sliceBps: Int = 123, votes: [String] = [], pot potReply: StubTransport.Reply? = nil
    ) throws -> [String: [StubTransport.Reply]] {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.id = cabalID
        cabal.name = "AC Capital"
        cabal.memberCount = 9
        return [
            potPath: [potReply ?? .json(.ok, pot(sliceBps: sliceBps))],
            cabalPath: [.json(.ok, try encode(cabal))],
            votesPath: [.json(.ok, "[" + votes.joined(separator: ",") + "]")],
            membersPath: [.json(.ok, try encode(Components.Schemas.CabalMemberPage.sampleACCapital))],
        ]
    }

    private func model(
        _ routes: [String: [StubTransport.Reply]], viewer: String? = nil
    ) -> (FirstFundModel, StubTransport) {
        let transport = StubTransport(routes: routes)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = FirstFundModel(
            api: api, cabalID: cabalID, viewerID: viewer ?? zoeID, settledMicros: 250_000_000)
        return (model, transport)
    }

    func testLoadReadsTheFourReadsOnceEach() async throws {
        let (model, transport) = model(try routes())

        await model.load()

        let sent = await transport.sent.compactMap(\.path)
        XCTAssertEqual(sent.count, 4)
        XCTAssertEqual(Set(sent), [potPath, cabalPath, votesPath, membersPath])
        XCTAssertEqual(model.value?.memberCount, 9)
        XCTAssertEqual(model.value?.potValueMicros, 20_250_000_000)
        XCTAssertEqual(model.value?.settledMicros, 250_000_000)
    }

    func testTheShareReadsInUpToTwoDecimalsWithoutFloatingPoint() async {
        XCTAssertEqual(FirstFundValue.shareText(bps: 123), "1.23%")
        XCTAssertEqual(FirstFundValue.shareText(bps: 3500), "35%")
        XCTAssertEqual(FirstFundValue.shareText(bps: 150), "1.5%")
        XCTAssertEqual(FirstFundValue.shareText(bps: 5), "0.05%")
        XCTAssertEqual(FirstFundValue.shareText(bps: 10_000), "100%")
    }

    func testTheShareLineComesFromThePotRead() async throws {
        let (model, _) = model(try routes(sliceBps: 123))

        await model.load()

        XCTAssertEqual(model.value?.shareBps, 123)
        XCTAssertEqual(model.value?.shareText, "1.23%")
    }

    func testTheFirstVoteIsTheOneInThisCabalNotAnotherCabals() async throws {
        let other = anotherCabalsVote
        let mine = vote(cabal: cabalID, id: "01890a5d-ac96-774b-bcce-b302099a8072", closes: "2026-10-29T15:00:00Z")
        let later = vote(cabal: cabalID, id: "01890a5d-ac96-774b-bcce-b302099a8073", closes: "2026-10-29T18:00:00Z")
        let (model, _) = model(try routes(votes: [other, mine, later]))

        await model.load()

        XCTAssertEqual(model.value?.vote?.proposalID, "01890a5d-ac96-774b-bcce-b302099a8072")
        XCTAssertEqual(model.value?.vote?.yes, 4)
        XCTAssertEqual(model.value?.vote?.no, 1)
        XCTAssertEqual(model.value?.vote?.voters, 9)
        XCTAssertEqual(model.value?.vote?.replies, 6)
    }

    func testNoVoteInThisCabalHidesTheSection() async throws {
        let other = anotherCabalsVote
        let (model, _) = model(try routes(votes: [other]))

        await model.load()

        XCTAssertNil(model.value?.vote)
    }

    func testTheStripPutsTheCrownFirstDropsTheViewerAndKeepsPageOrder() async throws {
        let logan = "01890a5d-ac96-774b-bcce-b302099a8073"
        let (model, _) = model(try routes(), viewer: logan)

        await model.load()

        XCTAssertEqual(
            model.value?.members.map(\.name), ["Christelle", "Andres", "Mia", "Jordan", "Priya", "Sam", "Kai"])
        XCTAssertEqual(model.value?.members.map(\.crown), [true] + Array(repeating: false, count: 6))
    }

    func testTheStripHoldsAtMostTwelve() async {
        var rows = Components.Schemas.CabalMemberPage.sampleACCapital.items
        let first = rows[0]
        for index in 0..<10 {
            var row = first
            row.userId = "01890a5d-ac96-774b-bcce-b302099a81\(index)0"
            rows.append(row)
        }

        XCTAssertEqual(FirstFundModel.strip(rows, viewerID: zoeID).count, 12)
    }

    func testAFailedPotReadShowsTheSettledAmountWithoutAShare() async throws {
        let (model, _) = model(try routes(pot: .json(.internalServerError, "{}")))

        await model.load()

        XCTAssertNil(model.value?.shareBps)
        XCTAssertNil(model.value?.shareText)
        XCTAssertNil(model.value?.potValueMicros)
        XCTAssertEqual(model.value?.settledMicros, 250_000_000)
    }

    func testAFailedCabalReadFailsTheScreenSoItCanRetry() async throws {
        var routes = try routes()
        routes[cabalPath] = [.json(.internalServerError, "{}")]
        let (model, _) = model(routes)

        await model.load()

        guard case .failed = model.state else { return XCTFail("expected failed, got \(model.state)") }
        XCTAssertNil(model.value)
    }

    func testTheHandOffConditionIsHoldingNoShareBeforeTheFund() async {
        XCTAssertTrue(FirstFundModel.heldNoShare(sliceBps: 0))
        XCTAssertTrue(FirstFundModel.heldNoShare(sliceBps: nil))
        XCTAssertFalse(FirstFundModel.heldNoShare(sliceBps: 1))
        XCTAssertFalse(FirstFundModel.heldNoShare(sliceBps: 3500))
    }
}
