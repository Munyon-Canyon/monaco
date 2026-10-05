import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalFailedSwapFixtureTests: XCTestCase {
    func testFailedSwapFixtureMapsRetryableBothWays() {
        for retryable in [true, false] {
            let detail = ProposalDetail(.failedSwap(retryable: retryable))
            XCTAssertEqual(detail.summary.status, .passed)
            XCTAssertEqual(detail.summary.swap?.status, "failed")
            XCTAssertEqual(detail.summary.swap?.retryable, retryable)
            XCTAssertEqual(detail.summary.swap?.failureMessage, "The trade did not go through.")
            XCTAssertEqual(detail.voters.map(\.ballot), ["yes", "yes", nil])
        }
    }
}

@MainActor
final class ProposalSampleModelTests: XCTestCase {
    func testSampleModelShowsTheFixtureWithoutLoading() async {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = ProposalDetailModel(
            sample: .failedSwap(retryable: true),
            members: Components.Schemas.Cabal.sampleWithMembers(role: "member").members, asset: .googl,
            repository: ProposalsRepository(api: api), hints: FakeHintStream())
        XCTAssertEqual(model.value?.summary.swap?.retryable, true)
        XCTAssertEqual(model.members.count, 3)
        XCTAssertEqual(model.asset?.decimals, 8)
    }
}
