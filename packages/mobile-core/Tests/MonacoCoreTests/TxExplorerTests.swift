import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class TxExplorerTests: XCTestCase {
    private let evmHash = "0x3f1c9d0a7b5e4c2d8a6f1b0e9d7c5a3b1f2e4d6c8a0b9e7d5c3a1f2e4d6c8a0b"

    func testASolanaSignatureOpensSolscan() {
        let link = TxExplorer.link(signature: "abc123", network: nil)

        XCTAssertEqual(link?.name, "Solscan")
        XCTAssertEqual(link?.url.absoluteString, "https://solscan.io/tx/abc123")
        XCTAssertEqual(TxExplorer.solscanURL(signature: "abc123")?.absoluteString, "https://solscan.io/tx/abc123")
    }

    func testAnEVMHashOpensThatChainsExplorerAndNeverSolscan() {
        let link = TxExplorer.link(signature: evmHash, network: .base)

        XCTAssertEqual(link?.name, "Basescan")
        XCTAssertEqual(link?.url.absoluteString, "https://basescan.org/tx/\(evmHash)")
        XCTAssertEqual(TxExplorer.link(signature: evmHash, network: .ethereum)?.name, "Etherscan")
        XCTAssertEqual(TxExplorer.link(signature: evmHash, network: .arbitrum)?.name, "Arbiscan")
        XCTAssertNil(TxExplorer.solscanURL(signature: evmHash))
    }

    func testAnEVMHashOnAChainWithNoKnownExplorerHasNoLink() {
        XCTAssertNil(TxExplorer.link(signature: evmHash, network: .robinhood))
        XCTAssertNil(TxExplorer.link(signature: evmHash, network: .solana))
        XCTAssertNil(TxExplorer.link(signature: evmHash, network: nil))
    }
}
