import MonacoAPI
import MonacoCore
import XCTest

final class AssetSeedTests: XCTestCase {
    func testSeedsFromAMarketAsset() {
        let asset = MarketMapping.detail(.googl).asset

        let seed = AssetSeed(asset)

        XCTAssertEqual(seed.name, asset.name)
        XCTAssertEqual(seed.ticker, asset.ticker)
        XCTAssertEqual(seed.priceMicros, asset.priceMicros)
        XCTAssertEqual(seed.changeBasisPoints, asset.changeBasisPoints)
    }
}
