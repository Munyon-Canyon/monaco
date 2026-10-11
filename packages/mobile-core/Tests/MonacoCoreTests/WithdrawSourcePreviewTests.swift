import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class WithdrawSourcePreviewTests: XCTestCase {
    private let evmAddress = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

    func testAnEVMPreviewPostsTheWithdrawalAndReturnsTheEstimateAndItsExpiry() async throws {
        let expiresAt = Date(timeIntervalSince1970: Date().timeIntervalSince1970.rounded(.down) + 300)
        let stamp = ISO8601DateFormatter().string(from: expiresAt)
        let transport = StubTransport(.json(.ok, #"{"receive_micros":"9992500","expires_at":"\#(stamp)"}"#))

        let preview = try await source(transport).preview(micros: 10_000_000, toAddress: evmAddress, network: .base)

        XCTAssertEqual(preview, WithdrawPreview(receiveMicros: 9_992_500, expiresAt: expiresAt))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertEqual(sent.map(\.path), ["/v1/me/withdrawals/preview"])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        let wire = try JSONSerialization.jsonObject(with: body) as? [String: String]
        XCTAssertEqual(
            wire,
            ["amount_micros": "10000000", "destination_kind": "evm", "network": "base", "to_address": evmAddress])
    }

    func testASolanaPreviewHasNoExpiry() async throws {
        let transport = StubTransport(.json(.ok, #"{"receive_micros":"10000000","expires_at":null}"#))

        let preview = try await source(transport).preview(
            micros: 10_000_000, toAddress: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P", network: .solana)

        XCTAssertEqual(preview, WithdrawPreview(receiveMicros: 10_000_000, expiresAt: nil))
    }

    func testARefusedPreviewThrows() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Unavailable", status: 503, code: .privyUnavailable,
                message: "Try again.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true))

        do {
            _ = try await source(transport).preview(micros: 10_000_000, toAddress: evmAddress, network: .arbitrum)
            XCTFail("a 503 preview returned an estimate")
        } catch {}
    }

    func testAnAmountThatIsNotMicrosIsADecodingError() async {
        let transport = StubTransport(.json(.ok, #"{"receive_micros":"-1","expires_at":null}"#))

        do {
            _ = try await source(transport).preview(micros: 10_000_000, toAddress: evmAddress, network: .ethereum)
            XCTFail("a negative estimate decoded")
        } catch {
            guard case APIError.decoding = error else { return XCTFail("\(error) is not a decoding error") }
        }
    }

    private func source(_ transport: StubTransport) -> WithdrawSource {
        WithdrawSource(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
    }
}
