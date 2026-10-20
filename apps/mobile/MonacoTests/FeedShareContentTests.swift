import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct FeedShareContentTests {
    private func items() async -> [Components.Schemas.FeedItem] {
        let model = FeedModel.preview(.items, clock: ContinuousClock())
        await model.load()
        return model.items
    }

    @Test func everyFeedItemSharesItsHeadlineOnMonaco() async {
        for item in await items() {
            let text = FeedShareContent.text(for: item)

            #expect(text.hasPrefix(item.title))
            #expect(text.hasSuffix(" on Monaco"))
        }
    }

    @Test func aCabalItemNamesTheCabalInTheText() async throws {
        let item = try #require(await items().first { $0.cabalName != nil })
        let name = try #require(item.cabalName)

        #expect(FeedShareContent.text(for: item).contains(" in \(name)"))
    }

    @Test func withoutAnInviteLinkTheShareFallsBackToTheWebsite() {
        #expect(FeedShareContent.url(links: nil) == ShareCardContent.fallbackURL)
    }
}
