import Foundation
@preconcurrency import GiphyUISDK
import MonacoCore
import Testing

@testable import Monaco

nonisolated struct GiphyConfigTests {
    @MainActor @Test func anEmptyOrUnexpandedValueReadsAsEmpty() {
        let cases: [(raw: Any?, key: String)] = [
            (nil, ""),
            ("", ""),
            ("   ", ""),
            ("$(GIPHY_SDK_KEY)", ""),
            (" $(GIPHY_SDK_KEY_DEV) ", ""),
            (42, ""),
            (" abc123 ", "abc123"),
        ]

        for row in cases {
            #expect(Config.giphyKey(from: row.raw) == row.key, "\(String(describing: row.raw))")
        }
    }

    @MainActor
    @Test func theProviderFollowsTheKeyAndTheTestFlag() {
        #expect(AppEnvironment.makeGifProvider(key: "k", isRunningTests: false) is GiphyGifProvider)
        #expect(AppEnvironment.makeGifProvider(key: "", isRunningTests: false) is UnavailableGifProvider)
        #expect(AppEnvironment.makeGifProvider(key: "k", isRunningTests: true) is SampleGifProvider)
    }

    @Test func aRequestAsksForPG13AtTwentyFiveAnOffset() {
        let config = GiphyGifProvider.config(
            apiKey: "test-key", path: "gifs/search", query: [URLQueryItem(name: "q", value: "rocket")], offset: 50)

        let items = Dictionary(uniqueKeysWithValues: (config.queryItems ?? []).map { ($0.name, $0.value) })
        #expect(config.path == "gifs/search")
        #expect(items["rating"] == "pg-13")
        #expect(items["limit"] == "25")
        #expect(items["offset"] == "50")
        #expect(items["q"] == "rocket")
        #expect(config.apiKey == "test-key")
        #expect(config.getRequest().url?.query?.contains("api_key=test-key") == true)
    }

    @Test func noKeyMeansUnavailable() async {
        let provider = GiphyGifProvider(apiKey: "")

        await #expect(throws: GifProviderError.unavailable) { try await provider.trending(offset: 0) }
        await #expect(throws: GifProviderError.unavailable) { try await provider.search("rocket", offset: 0) }
    }

    @Test func aPageMapsTheThreeRenditionsAndKeepsTheAttribution() throws {
        let page = Self.page([Self.item(id: "a"), Self.item(id: "b")], total: 2)

        #expect(page.items.map(\.id) == ["a", "b"])
        #expect(page.attribution == "Powered by GIPHY")
        #expect(page.nextOffset == nil)
        let first = try #require(page.items.first)
        #expect(first.still.url.lastPathComponent == "a-still.gif")
        #expect(first.preview.url.lastPathComponent == "a-200w.gif")
        #expect(first.original.url.lastPathComponent == "a-downsized.gif")
        #expect(first.preview.width == 200)
        #expect(first.preview.height == 150)
    }

    @Test func theOriginalRenditionStandsInForAMissingDownsized() throws {
        let item = Self.item(id: "a", images: ["fixed_width_still", "fixed_width", "original"])

        let page = Self.page([item])

        #expect(page.items.first?.original.url.lastPathComponent == "a-giphy.gif")
        #expect(page.items.first?.original.width == 480)
    }

    @Test func aResultMissingARenditionOrOffTheCDNIsDropped() throws {
        let items = [
            Self.item(id: "evil", hosts: ["fixed_width": "evil.example.com"]),
            Self.item(id: "look", hosts: ["fixed_width_still": "media.giphy.com.evil.test"]),
            Self.item(id: "nostill", images: ["fixed_width", "downsized"]),
            Self.item(id: "ok", hosts: ["original": "i.giphy.com"]),
            Self.item(id: "fine"),
        ]

        let page = Self.page(items)

        #expect(page.items.map(\.id) == ["ok", "fine"])
    }

    @Test func theNextOffsetAdvancesUntilTheTotal() throws {
        let items = [Self.item(id: "a"), Self.item(id: "b")]

        let more = Self.page(items, total: 10, offset: 4)
        let last = Self.page(items, total: 6, offset: 4)

        #expect(more.nextOffset == 6)
        #expect(last.nextOffset == nil)
    }

    @Test func theCDNRuleAcceptsOnlyMediaHostsAndIGiphy() throws {
        let accepted = ["https://media0.giphy.com/a.gif", "https://media.giphy.com/a.gif", "https://i.giphy.com/a.gif"]
        let refused = [
            "http://media0.giphy.com/a.gif", "https://giphy.com/a.gif", "https://x.media0.giphy.com/a.gif",
            "https://media0.giphy.com.evil.test/a.gif", "https://mediagiphy.com/a.gif", "https://cdn.giphy.com/a.gif",
        ]

        for string in accepted { #expect(GiphyGifProvider.isGiphyCDN(try #require(URL(string: string))), "\(string)") }
        for string in refused { #expect(!GiphyGifProvider.isGiphyCDN(try #require(URL(string: string))), "\(string)") }
    }

    private static let allImages = ["fixed_width_still", "fixed_width", "downsized", "original"]

    private static func item(
        id: String,
        images: [String] = allImages,
        hosts: [String: String] = [:]
    ) -> [String: Any] {
        let files = [
            "fixed_width_still": ("\(id)-still.gif", "200", "150"),
            "fixed_width": ("\(id)-200w.gif", "200", "150"),
            "downsized": ("\(id)-downsized.gif", "300", "225"),
            "original": ("\(id)-giphy.gif", "480", "360"),
        ]
        var renditions: [String: Any] = [:]
        for name in images {
            guard let file = files[name] else { continue }
            renditions[name] = [
                "url": "https://\(hosts[name] ?? "media2.giphy.com")/media/\(file.0)",
                "width": file.1,
                "height": file.2,
            ]
        }
        return [
            "type": "gif", "id": id, "url": "https://giphy.com/gifs/\(id)", "title": "GIF \(id)",
            "rating": "pg-13", "images": renditions,
        ]
    }

    private static func page(_ items: [[String: Any]], total: Int = 100, offset: Int = 0) -> GifPage {
        let json: [String: Any] = [
            "data": items,
            "pagination": ["total_count": total, "count": items.count, "offset": offset],
            "meta": ["status": 200, "msg": "OK", "response_id": "r"],
        ]
        return GiphyGifProvider.page(from: json, offset: offset)
    }
}
