import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FundSourceListTests: XCTestCase {
    private func model(_ transport: StubTransport) -> FundSourceListModel {
        FundSourceListModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
    }

    private func portfolio() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(Components.Schemas.MyPortfolio.sample), as: UTF8.self)
    }

    func testRowsCarryTheNameAndTheOwnershipShare() async throws {
        let list = model(StubTransport(scripted: [.json(.ok, try portfolio())]))

        await list.load()

        XCTAssertTrue(list.isLoaded)
        XCTAssertEqual(list.rows.map(\.name), ["Alpha", "Sunday Investors"])
        XCTAssertEqual(list.rows.map(\.ownership), ["35% yours", "12% yours"])
        XCTAssertEqual(list.row(for: Components.Schemas.CabalRef.sampleBeta.id)?.name, "Sunday Investors")
        XCTAssertNil(list.row(for: "missing"))
    }

    func testACabalWithoutASnapshotTakesTheNameTheOpeningScreenPassed() async throws {
        let list = model(StubTransport(scripted: [.json(.ok, try portfolio())]))
        await list.load()

        XCTAssertEqual(list.name(for: "fresh", opener: ("fresh", "Big Ballers")), "Big Ballers")
        XCTAssertEqual(list.name(for: "fresh", opener: ("fresh", "  ")), FundSourceCopy.fallbackName)
        XCTAssertEqual(list.name(for: "fresh", opener: ("other", "Big Ballers")), FundSourceCopy.fallbackName)
        XCTAssertEqual(list.name(for: "fresh", opener: nil), FundSourceCopy.fallbackName)
        let known = Components.Schemas.CabalRef.sampleBeta.id
        XCTAssertEqual(list.name(for: known, opener: (known, "Stale name")), "Sunday Investors")
    }

    func testAFailedReadLeavesNoRowsButIsLoaded() async {
        let list = model(StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))]))

        await list.load()

        XCTAssertTrue(list.isLoaded)
        XCTAssertTrue(list.rows.isEmpty)
    }

    func testTheCopyReadsWholePercentsAndCash() {
        XCTAssertEqual(FundSourceCopy.yours(bps: 800), "8% yours")
        XCTAssertEqual(FundSourceCopy.yours(bps: 0), "0% yours")
        XCTAssertEqual(FundSourceCopy.cash(micros: 500_000_000), "Cash $500.00")
    }
}
