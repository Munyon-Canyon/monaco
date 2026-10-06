import MonacoAPI
import MonacoCore
import XCTest

final class FeedMuteMatcherTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    private func removed(_ target: FeedMuteTarget) -> [String] {
        samples.filter { FeedMuteMatcher.removes(target, $0) }.map(\.id)
    }

    func testAKindMuteRemovesOnlyThatKind() {
        XCTAssertEqual(removed(.init(.kind, "trade")), [samples[1].id])
    }

    func testACabalMuteRemovesEveryItemOfThatCabalAndNotPriceMoves() {
        let cabal = Components.Schemas.FeedItem.sampleCabalID
        XCTAssertEqual(removed(.init(.cabal, cabal)), samples.filter { $0.cabalId == cabal }.map(\.id))
        XCTAssertFalse(removed(.init(.cabal, cabal)).contains(samples[2].id))
    }

    func testAnAssetMuteRemovesItemsAboutThatAsset() throws {
        let asset = try XCTUnwrap(samples[2].assetId)
        XCTAssertEqual(removed(.init(.asset, asset)), [samples[2].id])
    }

    func testAUserMuteRemovesThatActorsItems() throws {
        let actor = try XCTUnwrap(samples[3].actorId)
        XCTAssertEqual(removed(.init(.user, actor)), [samples[3].id])
    }

    func testAnItemMuteRemovesOnlyThatItem() {
        XCTAssertEqual(removed(.init(.item, samples[0].id)), [samples[0].id])
    }

    func testIdsMatchWithoutRegardToCase() {
        XCTAssertEqual(removed(.init(.item, samples[0].id.uppercased())), [samples[0].id])
    }

    func testAMuteForSomethingNoItemCarriesRemovesNothing() {
        XCTAssertEqual(removed(.init(.asset, "00000000-0000-7000-8000-00000000dead")), [])
        XCTAssertEqual(removed(.init(.kind, "unknown")), [])
    }

    func testTheMenuOffersEachTargetTheItemCarries() {
        let titles = FeedMuteOption.options(for: samples[1], viewerID: nil).map(\.menuTitle)
        XCTAssertEqual(
            titles,
            ["Hide this post", "Mute Weekend investors", "Mute NVDAx", "Mute Alex", "Mute Trades"])
    }

    func testTheMenuNeverOffersMutingYourself() throws {
        let viewer = try XCTUnwrap(samples[1].actorId)
        let titles = FeedMuteOption.options(for: samples[1], viewerID: viewer.uppercased()).map(\.menuTitle)
        XCTAssertFalse(titles.contains("Mute Alex"))
    }

    func testAPriceMoveOffersNoCabalAndNoMenuNamesAMintAddress() {
        let options = FeedMuteOption.options(for: samples[2], viewerID: nil)
        XCTAssertEqual(options.map(\.menuTitle), ["Hide this post", "Mute TSLAx", "Mute Price moves"])
        for option in options {
            XCTAssertFalse(option.menuTitle.contains(samples[2].assetId ?? "-"))
            XCTAssertFalse(option.toast.contains("xStock"))
        }
    }

    func testTheToastNamesWhatWasMuted() {
        let options = FeedMuteOption.options(for: samples[1], viewerID: nil)
        XCTAssertEqual(
            options.map(\.toast),
            ["Hidden.", "Muted Weekend investors.", "Muted NVDAx.", "Muted Alex.", "Muted Trades."])
    }
}
