import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalDetailModelColdLoadTests: XCTestCase {
    private func transport() throws -> StubTransport {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let detail = Components.Schemas.ProposalDetail.sample(canVote: true)
        let body = String(decoding: try encoder.encode(detail), as: UTF8.self)
        return StubTransport(.json(.ok, body))
    }

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    func testTheIdOnlyInitSubscribesToNoCabalHintsUntilTheFirstLoadResolvesTheCabal() async throws {
        let hints = FakeHintStream()
        let model = ProposalDetailModel(id: "proposal-1", repository: repository(try transport()), hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        for _ in 0..<50 { await Task.yield() }
        var count = await hints.subscriberCount
        XCTAssertEqual(count, 0)
        await model.load()
        await hints.waitForSubscribers(2)
        count = await hints.subscriberCount
        XCTAssertEqual(count, 2)
    }

    func testTheIdOnlyInitFetchesTheMembersAndAssetFromTheLoadedCabal() async throws {
        let transport = try transport()
        let model = ProposalDetailModel(id: "proposal-1", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        let cabalID = try XCTUnwrap(model.summary?.cabalID)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.first, "/v1/proposals/proposal-1")
        XCTAssertTrue(paths.contains("/v1/cabals/\(cabalID)"))
    }

    func testTheCabalInitSubscribesToTheCabalHintsFromTheStart() async throws {
        let hints = FakeHintStream()
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: repository(try transport()), hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        await hints.waitForSubscribers(2)
        let count = await hints.subscriberCount
        XCTAssertEqual(count, 2)
    }
}
