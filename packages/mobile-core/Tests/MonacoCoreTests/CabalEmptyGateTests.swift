import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalEmptyGateTests: XCTestCase {
    private static let cabalID = "cabal-1"
    private static let closedPath = "/v1/cabals/cabal-1/proposals?filter=closed&limit=1"

    func testAFundedCabalWithNoProposalsIsEmpty() {
        XCTAssertTrue(CabalEmptyGate.isEmpty(cabal: cabal(), hasClosedProposal: false, potState: .cashOnly))
    }

    func testAClosedProposalEndsItEvenWithNoPositions() {
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: cabal(), hasClosedProposal: true, potState: .cashOnly))
    }

    func testAnOpenProposalEndsIt() {
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: cabal(open: 1), hasClosedProposal: false, potState: .cashOnly))
    }

    func testAZeroPotIsNotEmpty() {
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: cabal(), hasClosedProposal: false, potState: .zero))
    }

    func testNothingShowsBeforeTheReadsLand() {
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: nil, hasClosedProposal: false, potState: .cashOnly))
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: cabal(), hasClosedProposal: nil, potState: .cashOnly))
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: cabal(), hasClosedProposal: false, potState: nil))
    }

    func testANonMemberNeverSeesIt() {
        XCTAssertFalse(CabalEmptyGate.isEmpty(cabal: .sample(role: nil), hasClosedProposal: false, potState: .invested))
    }

    func testTheSentenceUsesTheShortNameAndThePot() {
        XCTAssertEqual(
            CabalEmptyGate.sentence(cabalName: "Garage Fund Management", potValue: "$600.00"),
            "Garage Fund has $600.00 and no plan yet. The first proposal starts it.")
    }

    func testProposeBuyNeedsCanPropose() {
        XCTAssertTrue(CabalEmptyGate.showsPropose(canPropose: true))
        XCTAssertFalse(CabalEmptyGate.showsPropose(canPropose: false))
        XCTAssertFalse(CabalEmptyGate.showsPropose(canPropose: nil))
    }

    func testTheModelReadsOneClosedProposalAndFollowsTheAnswer() async throws {
        let none = try Self.page([])
        let one = try Self.page([.sample(status: .executed)])
        let (model, transport, _) = make([
            .json(.ok, none), .json(.ok, one), .failure(URLError(.notConnectedToInternet)),
        ])

        XCTAssertNil(model.hasClosedProposal)
        await model.load()
        XCTAssertEqual(model.hasClosedProposal, false)
        await model.load()
        XCTAssertEqual(model.hasClosedProposal, true)
        await model.load()
        XCTAssertEqual(model.hasClosedProposal, true)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, Array(repeating: Self.closedPath, count: 3))
    }

    func testAHintReloadsTheClosedRead() async throws {
        let (model, _, hints) = make([.json(.ok, try Self.page([.sample(status: .executed)]))])
        model.setVisible(true)
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(1)

        await hints.send(.changed(.cabal(Self.cabalID), what: "proposals", id: "1"))

        for _ in 0..<2000 where model.hasClosedProposal != true {
            await Task.yield()
        }
        XCTAssertEqual(model.hasClosedProposal, true)
        observer.cancel()
    }

    private func cabal(open: Int32 = 0) -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.openProposalCount = open
        return cabal
    }

    private func make(_ replies: [StubTransport.Reply]) -> (CabalEmptyModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(routes: [Self.closedPath: replies])
        let hints = FakeHintStream()
        let model = CabalEmptyModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
        return (model, transport, hints)
    }

    private static func page(_ proposals: [Components.Schemas.Proposal]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(
            decoding: try encoder.encode(Components.Schemas.ProposalList(proposals: proposals, nextCursor: nil)),
            as: UTF8.self)
    }
}
