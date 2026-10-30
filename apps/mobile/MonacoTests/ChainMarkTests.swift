import Testing
import UIKit

@testable import Monaco

struct ChainMarkTests {
    @Test(arguments: ["solana", "base", "robinhood"])
    func everyChainADepositOrTradeCanUseHasItsMarkInTheCatalog(network: String) throws {
        let name = try #require(ChainMark.imageName(for: network), "\(network) has no mark")
        #expect(UIImage(named: name) != nil, "\(name) is missing from the catalog")
    }

    @Test(arguments: ["polygon", "ethereum", "arbitrum", "monad"])
    func aChainWithoutAMarkFallsBackToTheGenericGlyph(network: String) {
        #expect(ChainMark.imageName(for: network) == nil)
    }
}
