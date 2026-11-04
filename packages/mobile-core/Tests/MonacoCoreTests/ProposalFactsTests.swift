import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalFactsTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_792_662_780)

    private func facts(
        _ fixture: Components.Schemas.ProposalDetail, asset: Components.Schemas.AssetDetail = .nvda
    ) -> ProposalFacts? {
        ProposalFacts.make(detail: ProposalDetail(fixture), asset: ProposalAsset(asset))
    }

    func testAnOpenBuyReadsAsOneSentenceAndTheTreasuryShare() async throws {
        let result = try XCTUnwrap(facts(.openNVDA(now: now)))
        XCTAssertEqual(result.estimate, "About 1.82 shares at $220.00")
        XCTAssertEqual(result.info, .price)
        XCTAssertEqual(result.treasury, "2% of treasury")
    }

    func testEveryNonFilledStatusReadsTheSame() async throws {
        let statuses: [Components.Schemas.ProposalStatus] = [.open, .passed, .failed, .expired, .withdrawn, .voided]
        for status in statuses {
            var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
            fixture.status = status
            let result = try XCTUnwrap(facts(fixture), "\(status)")
            XCTAssertEqual(result.estimate, "About 1.82 shares at $220.00", "\(status)")
            XCTAssertEqual(result.treasury, "2% of treasury", "\(status)")
        }
    }

    func testAPassedBuyThatHasNotFilledStillReads() async throws {
        XCTAssertEqual(facts(.passedNVDA(now: now))?.estimate, "About 1.82 shares at $220.00")
    }

    func testAnExactQuantityDropsAbout() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.sharesExact = true
        XCTAssertEqual(facts(fixture)?.estimate, "1.82 shares at $220.00")
    }

    func testASellReadsExactShares() async throws {
        let result = try XCTUnwrap(facts(.failedTSLA(now: now), asset: .nvda))
        XCTAssertEqual(result.estimate, "2.2 shares at $412.55")
        XCTAssertEqual(result.info, .price)
        XCTAssertEqual(result.treasury, "4% of treasury")
    }

    func testAPreIpoBuyReadsTheEstimate() async throws {
        let result = try XCTUnwrap(facts(.preIpoSpaceX(now: now), asset: .spaceX))
        XCTAssertEqual(result.estimate, "About 2.70 shares at an estimated $185.00")
        XCTAssertEqual(result.info, .estimate)
        XCTAssertEqual(result.treasury, "2.2% of treasury")
    }

    func testAPreIpoAssetWithoutRoundFactsReadsAsAPrice() async throws {
        var asset = Components.Schemas.AssetDetail.spaceX
        asset.preIpo = nil
        let result = try XCTUnwrap(facts(.preIpoSpaceX(now: now), asset: asset))
        XCTAssertEqual(result.info, .price)
        XCTAssertFalse(try XCTUnwrap(result.estimate).contains("estimated"))
    }

    func testAPreIpoBuyWithoutSharesOrPriceNamesTheEstimate() async throws {
        var fixture = Components.Schemas.ProposalDetail.preIpoSpaceX(now: now)
        fixture.usdcMicros = nil
        fixture.stats.sharesAtomics = nil
        XCTAssertEqual(facts(fixture, asset: .spaceX)?.estimate, "An estimated $185.00 a share")
        XCTAssertEqual(facts(fixture, asset: .spaceX)?.info, .estimate)
    }

    func testAMissingPriceLeavesItOut() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.priceMicros = nil
        let result = try XCTUnwrap(facts(fixture))
        XCTAssertEqual(result.estimate, "About 1.82 shares")
        XCTAssertEqual(result.info, .price)
    }

    func testMissingSharesReadsThePricePerShare() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.sharesAtomics = nil
        XCTAssertEqual(facts(fixture)?.estimate, "$220.00 a share")
    }

    func testNeitherFigureGivesNoEstimateAndNoInfo() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.priceMicros = nil
        fixture.stats.sharesAtomics = nil
        let result = try XCTUnwrap(facts(fixture))
        XCTAssertNil(result.estimate)
        XCTAssertNil(result.info)
        XCTAssertEqual(result.spoken, "2 percent of treasury.")
    }

    func testAMissingTreasuryShareIsNil() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.treasuryShareBps = nil
        let result = try XCTUnwrap(facts(fixture))
        XCTAssertNil(result.treasury)
        XCTAssertEqual(result.spoken, "About 1.82 shares at $220.00.")
    }

    func testOneShareReadsInTheSingular() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.sharesAtomics = 100_000_000
        XCTAssertEqual(facts(fixture)?.estimate, "About 1 share at $220.00")
    }

    func testGroupedSharesReadInThePlural() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.sharesAtomics = 123_400_000_000
        XCTAssertEqual(facts(fixture)?.estimate, "About 1,234 shares at $220.00")
    }

    func testAPreIpoOneShareReadsInTheSingular() async throws {
        var fixture = Components.Schemas.ProposalDetail.preIpoSpaceX(now: now)
        fixture.usdcMicros = 185_000_000
        XCTAssertEqual(facts(fixture, asset: .spaceX)?.estimate, "About 1.00 share at an estimated $185.00")
    }

    func testATinyTreasuryShareReadsLessThanAndSpeaksIt() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.stats.treasuryShareBps = 4
        let result = try XCTUnwrap(facts(fixture))
        XCTAssertEqual(result.treasury, "<0.1% of treasury")
        XCTAssertEqual(result.spoken, "About 1.82 shares at $220.00. Less than 0.1 percent of treasury.")
    }

    func testTheSpokenFormSpellsOutPercent() async throws {
        XCTAssertEqual(
            facts(.openNVDA(now: now))?.spoken, "About 1.82 shares at $220.00. 2 percent of treasury.")
    }

    func testARuleChangeHasNoFacts() async throws {
        XCTAssertNil(facts(.ruleSample(now: now)))
    }

    func testAFilledTradeHasNoFacts() async throws {
        XCTAssertNil(facts(.filledNVDA(now: now)))
    }

    func testNoOutputEverShowsADashOrAnApproximation() async throws {
        var bare = Components.Schemas.ProposalDetail.openNVDA(now: now)
        bare.stats.priceMicros = nil
        bare.stats.sharesAtomics = nil
        bare.stats.treasuryShareBps = nil
        let fixtures: [Components.Schemas.ProposalDetail] = [
            .openNVDA(now: now), .passedNVDA(now: now), .failedTSLA(now: now), .preIpoSpaceX(now: now), bare,
        ]
        for fixture in fixtures {
            let result = facts(fixture, asset: .spaceX)
            let text = [result?.estimate, result?.treasury, result?.spoken].compactMap { $0 }.joined()
            XCTAssertFalse(text.contains("—"), text)
            XCTAssertFalse(text.contains("≈"), text)
        }
    }
}
