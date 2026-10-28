import Foundation
import MonacoCore
import XCTest

final class GifProviderTests: XCTestCase {
    func testTheSamplePageHasEightGifsWithVariedShapesAndTheAttribution() async throws {
        let page = try await SampleGifProvider().trending(offset: 0)

        XCTAssertEqual(page.items.count, 8)
        XCTAssertEqual(Set(page.items.map(\.id)).count, 8)
        XCTAssertGreaterThan(Set(page.items.map { $0.preview.width * 1000 / $0.preview.height }).count, 4)
        XCTAssertEqual(page.attribution, "Powered by GIPHY")
        XCTAssertNil(page.nextOffset)
    }

    func testSampleMediaStaysOffTheRealGiphyHosts() async throws {
        let page = try await SampleGifProvider().trending(offset: 0)

        for gif in page.items {
            for media in [gif.still, gif.preview, gif.original] {
                XCTAssertEqual(media.url.host, SampleGifProvider.host)
            }
        }
    }

    func testASearchReturnsTheSamplePageAndALaterOffsetReturnsNone() async throws {
        let provider = SampleGifProvider()

        let first = try await provider.search("rocket", offset: 0)
        let later = try await provider.search("rocket", offset: 8)

        XCTAssertEqual(first.items.count, 8)
        XCTAssertTrue(later.items.isEmpty)
    }

    func testTheNoMatchesModeSearchesToAnEmptyPageButStillTrends() async throws {
        let provider = SampleGifProvider(mode: .noMatches)

        let search = try await provider.search("zzzz", offset: 0)
        let trending = try await provider.trending(offset: 0)

        XCTAssertTrue(search.items.isEmpty)
        XCTAssertNil(search.nextOffset)
        XCTAssertEqual(search.attribution, "Powered by GIPHY")
        XCTAssertEqual(trending.items.count, 8)
    }

    func testTheFailingModeThrowsFailed() async {
        let provider = SampleGifProvider(mode: .failing)

        await assertThrows(.failed) { _ = try await provider.trending(offset: 0) }
        await assertThrows(.failed) { _ = try await provider.search("rocket", offset: 0) }
    }

    func testTheUnavailableProviderThrowsUnavailable() async {
        let provider = UnavailableGifProvider()

        await assertThrows(.unavailable) { _ = try await provider.trending(offset: 0) }
        await assertThrows(.unavailable) { _ = try await provider.search("rocket", offset: 0) }
    }

    private func assertThrows(
        _ expected: GifProviderError,
        _ body: () async throws -> Void,
        file: StaticString = #filePath,
        line: UInt = #line
    ) async {
        do {
            try await body()
            XCTFail("Expected \(expected)", file: file, line: line)
        } catch let error as GifProviderError {
            XCTAssertEqual(error, expected, file: file, line: line)
        } catch {
            XCTFail("Unexpected error \(error)", file: file, line: line)
        }
    }
}
