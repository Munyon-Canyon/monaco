import MonacoCore
import XCTest

@testable import Monaco

nonisolated final class GiphyLiveTests: XCTestCase {
    @MainActor func testTrendingAndSearchReturnPG13MediaOnTheGiphyCDN() async throws {
        let key = Config.giphySDKKey
        try XCTSkipIf(key.isEmpty, "no GIPHY_SDK_KEY in this build")
        let provider = GiphyGifProvider(apiKey: key)

        let trending = try await provider.trending(offset: 0)
        let search = try await provider.search("rocket", offset: 0)

        for page in [trending, search] {
            XCTAssertFalse(page.items.isEmpty)
            XCTAssertLessThanOrEqual(page.items.count, 25)
            XCTAssertEqual(page.attribution, "Powered by GIPHY")
            for gif in page.items {
                for media in [gif.still, gif.preview, gif.original] {
                    XCTAssertTrue(GiphyGifProvider.isGiphyCDN(media.url), "\(media.url)")
                    XCTAssertGreaterThan(media.width, 0)
                    XCTAssertGreaterThan(media.height, 0)
                }
            }
        }
    }
}
