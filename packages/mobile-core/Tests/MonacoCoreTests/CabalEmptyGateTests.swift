import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalEmptyGateTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    private static var cabal: Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.openProposalCount = 0
        return cabal
    }

    private static var funded: CabalPotSummary { CabalPotSummary(.sampleCashOnly) }

    func testItIsOnForAFundedCabalThatNeverHadAProposal() {
        XCTAssertTrue(CabalEmptyGate.isOn(cabal: Self.cabal, pot: Self.funded, history: .none))
    }

    func testItIsOffOnceTheCabalHasAClosedProposalEvenWithNoPositions() {
        let pot = Self.funded
        XCTAssertEqual(pot.state, .cashOnly)
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: Self.cabal, pot: pot, history: .some))
    }

    func testItIsOffWithAnOpenRuleProposal() {
        var cabal = Self.cabal
        cabal.openProposalCount = 1
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: cabal, pot: Self.funded, history: .none))
    }

    func testItIsOffWithAZeroPot() {
        let pot = CabalPotSummary(.sampleZero)
        XCTAssertEqual(pot.state, .zero)
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: Self.cabal, pot: pot, history: .none))
    }

    func testItIsOffUntilTheClosedReadAnswers() {
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: Self.cabal, pot: Self.funded, history: .unknown))
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: Self.cabal, pot: nil, history: .none))
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: nil, pot: Self.funded, history: .none))
    }

    func testItIsOffForANonMemberAndABannedCabal() {
        var outsider = Self.cabal
        outsider.me = nil
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: outsider, pot: Self.funded, history: .none))
        var banned = Self.cabal
        banned.status = "banned"
        XCTAssertFalse(CabalEmptyGate.isOn(cabal: banned, pot: Self.funded, history: .none))
    }

    func testTheCardSentenceUsesTheShortNameAndThePot() {
        XCTAssertEqual(CabalEmptyGate.headline(), "Quiet in here.")
        XCTAssertEqual(
            CabalEmptyGate.body(name: "Garage Fund", pot: "$600.00"),
            "Garage Fund has $600 and no plan yet. The first proposal starts it.")
        XCTAssertEqual(
            CabalEmptyGate.body(name: "AC Capital Management", pot: "$1,234.50"),
            "AC Capital has $1,234.50 and no plan yet. The first proposal starts it.")
    }

    func testAnEmptyClosedListMeansNoHistoryAndOneClosedReadIsMade() async throws {
        let empty = try Self.encode(Components.Schemas.ProposalList(proposals: [], nextCursor: nil))
        let transport = StubTransport(.json(.ok, empty))
        let model = makeModel(transport)

        await model.load()

        XCTAssertEqual(model.history, .none)
        XCTAssertTrue(model.isOn(cabal: Self.cabal, pot: Self.funded))
        let paths: [String] = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.count, 1)
        XCTAssertTrue(paths[0].contains("filter=closed"))
        XCTAssertTrue(paths[0].contains("limit=1"))
    }

    func testAClosedProposalMeansHistoryAndTurnsTheGateOff() async throws {
        let list = Components.Schemas.ProposalList(proposals: [.sample(status: .executed)], nextCursor: nil)
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(list))))

        await model.load()

        XCTAssertEqual(model.history, .some)
        XCTAssertFalse(model.isOn(cabal: Self.cabal, pot: Self.funded))
    }

    func testAFailedReadLeavesTheGateOff() async {
        let model = makeModel(StubTransport(.failure(URLError(.notConnectedToInternet))))

        await model.load()

        XCTAssertEqual(model.history, .unknown)
        XCTAssertFalse(model.isOn(cabal: Self.cabal, pot: Self.funded))
    }

    func testAProposalHintReadsTheClosedListAgain() async throws {
        let empty = try Self.encode(Components.Schemas.ProposalList(proposals: [], nextCursor: nil))
        let closed = try Self.encode(
            Components.Schemas.ProposalList(proposals: [.sample(status: .executed)], nextCursor: nil))
        let transport = StubTransport(scripted: [.json(.ok, empty), .json(.ok, closed)])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        model.setVisible(true)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        await hints.waitForSubscribers(2)

        await hints.send(.changed(.cabal(Self.cabalID), what: "proposal_updated", id: "p1"))

        let reloaded = await waitUntil { model.history == .some }
        XCTAssertTrue(reloaded)
    }

    private func waitUntil(_ predicate: @escaping () -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if predicate() { return true }
            await Task.yield()
        }
        return predicate()
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CabalEmptyModel {
        CabalEmptyModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
