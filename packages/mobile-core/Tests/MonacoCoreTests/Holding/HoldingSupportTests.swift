import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class HoldingSupportTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_996_400)

    func testEveryFixtureScenarioBuildsAModelInTheStateItNames() {
        for scenario in CabalHoldingFixture.Scenario.allCases {
            let model = CabalHoldingFixture.model(scenario, now: now)

            switch (scenario, model.outcome) {
            case (.loading, .loading), (.error, .failed), (.notMember, .notMember), (.empty, .empty): break
            case (.ready, .loaded), (.loss, .loaded), (.stale, .loaded), (.productionGaps, .loaded),
                (.tradesFailed, .loaded):
                break
            default: XCTFail("\(scenario) built \(model.outcome)")
            }
        }
    }

    func testTheReadyFixtureCarriesTradesAndAnOpenProposal() {
        let model = CabalHoldingFixture.model(.ready, now: now)

        guard case .loaded(let trades, true) = model.trades else { return XCTFail("got \(model.trades)") }
        XCTAssertEqual(trades.count, 3)
        XCTAssertEqual(model.proposals.count, 1)
        XCTAssertEqual(model.proposals.first?.symbol, CabalHoldingFixture.symbol)
        XCTAssertEqual(CabalHoldingFixture.model(.tradesFailed, now: now).trades, .failed)
        XCTAssertEqual(CabalHoldingFixture.model(.notMember, now: now).cabalName, CabalHoldingFixture.cabalName)
    }

    func testAForcedOfflinePathStaysOfflineAndTheLiveOneStartsOnline() {
        let offline = NetworkPathModel(forcedOffline: true)
        offline.start()
        XCTAssertFalse(offline.isOnline)
        offline.stop()

        let live = NetworkPathModel()
        XCTAssertTrue(live.isOnline)
        live.start()
        live.start()
        live.stop()
    }

    func testTheStoreBuiltOnTheAPIFetchesTheProposalAndNamesIt() async throws {
        let detail = Components.Schemas.ProposalDetail.sample()
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(decoding: try encoder.encode(detail), as: UTF8.self)
        let transport = StubTransport(routes: ["/v1/proposals/\(detail.id)": [.json(.ok, body)]])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        let store = ChatProposalStore(api: api)

        await store.load(detail.id)

        guard case .loaded(let facts) = store.state(for: detail.id) else {
            return XCTFail("got \(store.state(for: detail.id))")
        }
        XCTAssertEqual(facts.phase, .open)
        XCTAssertEqual(facts.proposerID, detail.proposerId)
        XCTAssertFalse(facts.isSell)
    }
}
