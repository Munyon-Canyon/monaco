import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class SwapReceiptRouteTests: XCTestCase {
    private func swap(
        status: String, steps: String?, route: Bool = true, action: String = "buy", signature: String? = nil
    ) throws -> Components.Schemas.SwapDetail {
        let now = ISO8601DateFormatter().string(from: Date())
        let routeJSON =
            route
            ? """
            "route": {"winner_listing_id": "w", "quotes": [
              {"listing_id": "w", "issuer": "ondo", "network": "base", "venue": "uniswap", "net_share_nanos": "41000000",
               "net_micros": "10000000", "costs": [], "quoted_at": "\(now)", "expires_at": "\(now)"},
              {"listing_id": "s", "issuer": "xstocks", "network": "solana", "venue": "jupiter",
               "net_share_nanos": "40000000", "net_micros": "10000000", "costs": [], "quoted_at": "\(now)",
               "expires_at": "\(now)"}], "unavailable": []},
            """ : ""
        let stepsJSON = steps.map { "\"steps\": \($0)," } ?? ""
        let json = """
            {\(routeJSON) \(stepsJSON) "id": "swap-1", "cabal_id": "cabal-1",
             "source": {"kind": "proposal", "id": "proposal-1"}, "action": "\(action)", "symbol": "AAPLx",
             "asset_name": "Apple", "token_decimals": 8, "usdc_micros": 10000000, "token_amount": null,
             "status": "\(status)", "failure_code": null, "failure_message": null,
             "tx_signature": \(signature.map { "\"\($0)\"" } ?? "null"),
             "created_at": "\(now)", "confirmed_at": null, "retryable": false}
            """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(Components.Schemas.SwapDetail.self, from: Data(json.utf8))
    }

    private func step(_ name: String, _ status: String) -> String {
        let now = ISO8601DateFormatter().string(from: Date())
        return """
            {"step": "\(name)", "network": "base", "status": "\(status)", "amount_micros": "10000000",
             "updated_at": "\(now)"}
            """
    }

    func testARoutedTradeCarriesItsComparison() throws {
        let receipt = SwapReceipt(try swap(status: "confirmed", steps: nil))

        XCTAssertEqual(receipt.bestPrice?.headline, "Best price across 2 venues")
        XCTAssertEqual(receipt.bestPrice?.winner.name, "Base")
        XCTAssertEqual(receipt.statusLabel, "Done")
    }

    func testOpenStepsReadInProgressWithNoChainWords() throws {
        let steps = "[\(step("bridge_out", "succeeded")), \(step("swap", "submitted"))]"

        let receipt = SwapReceipt(try swap(status: "submitted", steps: steps))

        XCTAssertEqual(receipt.statusLabel, "In progress")
    }

    func testFinishedStepsAndFailuresKeepTheirStatus() throws {
        let done = "[\(step("bridge_out", "succeeded")), \(step("swap", "succeeded"))]"
        XCTAssertEqual(SwapReceipt(try swap(status: "confirmed", steps: done)).statusLabel, "Done")
        let open = "[\(step("bridge_out", "succeeded")), \(step("swap", "pending"))]"
        XCTAssertEqual(SwapReceipt(try swap(status: "failed", steps: open)).statusLabel, "Failed")
    }

    func testASolanaSwapWithNoRouteShowsNoComparison() throws {
        let receipt = SwapReceipt(try swap(status: "submitted", steps: nil, route: false, action: "sell"))

        XCTAssertNil(receipt.bestPrice)
        XCTAssertEqual(receipt.statusLabel, "Pending")
    }

    func testARoutedEVMTradeLinksItsHashToTheChainsExplorer() throws {
        let hash = "0x3f1c9d0a7b5e4c2d8a6f1b0e9d7c5a3b1f2e4d6c8a0b9e7d5c3a1f2e4d6c8a0b"
        let receipt = SwapReceipt(try swap(status: "confirmed", steps: nil, action: "sell", signature: hash))

        XCTAssertEqual(receipt.explorer?.name, "Basescan")
        XCTAssertEqual(receipt.explorer?.url.absoluteString, "https://basescan.org/tx/\(hash)")
    }

    func testASolanaSignatureLinksToSolscanAndAnUnroutedEVMHashToNothing() throws {
        let solana = SwapReceipt(try swap(status: "confirmed", steps: nil, route: false, signature: "5VERv8N"))
        let unrouted = SwapReceipt(try swap(status: "confirmed", steps: nil, route: false, signature: "0xabc"))

        XCTAssertEqual(solana.explorer?.url.absoluteString, "https://solscan.io/tx/5VERv8N")
        XCTAssertNil(unrouted.explorer)
    }
}
