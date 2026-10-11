import Testing
import UIKit

@testable import Monaco

struct ChainMarkTests {
    @Test(arguments: ["solana", "base", "ethereum", "arbitrum", "robinhood"])
    func everyChainADepositOrTradeCanUseHasItsMarkInTheCatalog(network: String) throws {
        let name = try #require(ChainMark.imageName(for: network), "\(network) has no mark")
        #expect(UIImage(named: name) != nil, "\(name) is missing from the catalog")
    }

    @Test func aChainWithoutAMarkFallsBackToTheGenericGlyph() {
        #expect(ChainMark.imageName(for: "polygon") == nil)
    }
}
