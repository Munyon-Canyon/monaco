import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct SharePayloadTests {
    private static let links = InviteLinks(Components.Schemas.MyReferralCode.sample)

    @Test func aCabalCardSharesItsTitleAndTheSharersInviteLink() {
        let content = ShareCardSamples.cabalGain
        let payload = SharePayload.card(content, image: nil)

        #expect(payload.title == content.title)
        #expect(payload.url == content.shareURL)
        #expect(payload.url == Self.links?.shareURL)
    }

    @Test func aReturnCardSharesItsTitleAndTheSharersInviteLink() {
        let content = ShareCardSamples.me(.gain)
        let payload = SharePayload.card(content, image: nil)

        #expect(payload.title == content.title)
        #expect(payload.url == Self.links?.shareURL)
    }

    @Test func aCardWithoutAnInviteLinkSharesTheWebsite() {
        let payload = SharePayload.card(ShareCardSamples.me(.gain, links: nil), image: nil)

        #expect(payload.url == ShareCardContent.fallbackURL)
    }

    @Test func aFeedItemSharesItsHeadlineAndTheInviteLink() async throws {
        let model = FeedModel.preview(.items, clock: ContinuousClock())
        await model.load()
        let item = try #require(model.items.first)
        let payload = FeedShareContent.payload(for: item, links: Self.links)

        #expect(payload.title == FeedShareContent.text(for: item))
        #expect(payload.url == Self.links?.shareURL)
        #expect(payload.image == nil)
    }

    @Test func aFeedItemWithoutAnInviteLinkSharesTheWebsite() async throws {
        let model = FeedModel.preview(.items, clock: ContinuousClock())
        await model.load()
        let item = try #require(model.items.first)

        #expect(FeedShareContent.payload(for: item, links: nil).url == ShareCardContent.fallbackURL)
    }
}
