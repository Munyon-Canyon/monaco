import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class DepositAddressesSourceTests: XCTestCase {
    private let solana = "5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP"
    private let evm = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

    func testASolanaOnlyAnswerListsSolanaAlone() async throws {
        let transport = StubTransport(.json(.ok, Self.body([("solana", solana)])))
        let source = DepositAddressesSource(api: api(transport))

        await source.load()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/deposit-addresses"])
        XCTAssertEqual(source.networks, [DepositNetwork(id: "solana", address: solana)])
        XCTAssertEqual(source.networks?.first?.displayName, "Solana")
        XCTAssertEqual(source.networks?.first?.isEVM, false)
    }

    func testSolanaAndBaseListSolanaFirst() async throws {
        let transport = StubTransport(.json(.ok, Self.body([("base", evm), ("solana", solana)])))
        let source = DepositAddressesSource(api: api(transport))

        await source.load()

        let networks = try XCTUnwrap(source.networks)
        XCTAssertEqual(networks.map(\.id), ["solana", "base", "monad"])
        XCTAssertEqual(networks.map(\.displayName), ["Solana", "Base", "Monad"])
        XCTAssertEqual(networks[1].address, evm)
        XCTAssertEqual(networks[1].isEVM, true)
    }

    func testNoEVMNetworkMeansNoMonadEntry() async throws {
        let json = #"{"addresses":[{"network":"solana","address":"\#(solana)","asset":"USDC"}]}"#

        XCTAssertEqual(try DepositNetwork.list(json: Data(json.utf8)).map(\.id), ["solana"])
    }

    func testAnUnknownNetworkStillShowsUnderItsOwnName() async throws {
        let transport = StubTransport(
            .json(.ok, Self.body([("solana", solana), ("monad", evm), ("arbitrum", evm)])))
        let source = DepositAddressesSource(api: api(transport))

        await source.load()

        let networks = try XCTUnwrap(source.networks)
        XCTAssertEqual(networks.map(\.id), ["solana", "monad", "arbitrum"])
        XCTAssertEqual(networks.map(\.displayName), ["Solana", "Monad", "Arbitrum"])
    }

    func testEntriesWithNoAddressOrAnotherAssetAreDropped() async throws {
        let json =
            #"{"addresses":[{"network":"solana","address":"\#(solana)","asset":"USDC"},"#
            + #"{"network":"base","address":"  ","asset":"USDC"},"#
            + #"{"network":"ethereum","address":"\#(evm)","asset":"USDT"}]}"#

        XCTAssertEqual(try DepositNetwork.list(json: Data(json.utf8)).map(\.id), ["solana"])
    }

    func testAnEVMItemCarriesItsEstimatedFee() async throws {
        let json =
            #"{"addresses":[{"network":"solana","address":"\#(solana)","asset":"USDC"},"#
            + #"{"network":"base","address":"\#(evm)","asset":"USDC","estimated_fee_micros":"163098"},"#
            + #"{"network":"arbitrum","address":"\#(evm)","asset":"USDC"}]}"#

        let networks = try DepositNetwork.list(json: Data(json.utf8))

        XCTAssertEqual(networks.map(\.estimatedFeeMicros), [nil, 163_098, nil, 163_098])
    }

    func testAMalformedFeeIsNilAndKeepsTheAddress() async throws {
        let json =
            #"{"addresses":[{"network":"base","address":"\#(evm)","asset":"USDC","estimated_fee_micros":"0.16"}]}"#

        let networks = try DepositNetwork.list(json: Data(json.utf8))

        XCTAssertEqual(
            networks, [DepositNetwork(id: "base", address: evm), DepositNetwork(id: "monad", address: evm)])
        XCTAssertNil(networks.first?.estimatedFeeMicros)
    }

    func testSolanaNeverCarriesAFee() async throws {
        let json =
            #"{"addresses":[{"network":"solana","address":"\#(solana)","asset":"USDC","estimated_fee_micros":"5000"}]}"#

        XCTAssertNil(try DepositNetwork.list(json: Data(json.utf8)).first?.estimatedFeeMicros)
    }

    func testABodyThatIsNotTheShapeIsADecodingError() {
        XCTAssertThrowsError(try DepositNetwork.list(json: Data(#"{"addresses":"nope"}"#.utf8))) { error in
            guard case APIError.decoding = error else { return XCTFail("\(error) is not a decoding error") }
        }
    }

    func testNotImplementedFallsBackToTheSolanaAddress() async throws {
        let transport = try StubTransport(.problem(problem(501, .notImplemented)))
        let source = DepositAddressesSource(api: api(transport))

        await source.load()

        guard case .failed = source.state else { return XCTFail("\(source.state) is not failed") }
        XCTAssertEqual(
            DepositAddressesSource.offered(source.state, solanaAddress: solana),
            [DepositNetwork(id: "solana", address: solana)])
    }

    func testAnEmptyListFallsBackToTheSolanaAddress() {
        XCTAssertEqual(
            DepositAddressesSource.offered(.loaded([]), solanaAddress: solana),
            [DepositNetwork(id: "solana", address: solana)])
    }

    func testResetClearsTheShownAddresses() async throws {
        let transport = StubTransport(.json(.ok, Self.body([("solana", solana), ("base", evm)])))
        let source = DepositAddressesSource(api: api(transport))
        await source.load()

        source.reset()

        XCTAssertNil(source.networks)
        XCTAssertEqual(source.state, .idle)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func problem(_ status: Int, _ code: Components.Schemas.ErrorCode) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Not implemented", status: status, code: code,
            message: "Not yet.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }

    static func body(_ entries: [(String, String)]) -> String {
        let items = entries.map { #"{"network":"\#($0.0)","address":"\#($0.1)","asset":"USDC"}"# }
        return #"{"addresses":[\#(items.joined(separator: ","))]}"#
    }
}
