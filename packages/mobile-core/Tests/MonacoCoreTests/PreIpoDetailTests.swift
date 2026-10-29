import MonacoAPI
import XCTest

@testable import MonacoCore

final class PreIpoDetailTests: XCTestCase {
    private let locale = Locale(identifier: "en_US")
    private let now = Date(timeIntervalSince1970: 1_793_000_000)

    private func detail(
        tradable: Bool = true, preIpo: Bool = true, edit: (inout Components.Schemas.AssetDetail) -> Void = { _ in }
    ) -> Components.Schemas.AssetDetail {
        var value = Components.Schemas.AssetDetail.spaceX
        value.tradable = tradable
        if !preIpo { value.preIpo = nil }
        edit(&value)
        return value
    }

    private func profile(_ value: Components.Schemas.AssetDetail) throws -> PreIpoDetail.Profile {
        try XCTUnwrap(PreIpoDetail(value, locale: locale)?.profile)
    }

    func testEveryStringOfTheFixtureReadsAsTheFrameDrawsIt() throws {
        let page = try profile(detail())

        XCTAssertEqual(page.estimateText, "$185.00")
        XCTAssertEqual(page.estimateCaption, "as of Oct 20")
        XCTAssertEqual(
            page.ledger,
            [
                PreIpoCopy.LedgerLine(label: "Last round · Jun ’26", value: "$165.00"),
                PreIpoCopy.LedgerLine(label: "Secondary estimate · Oct 20", value: "$185.00"),
            ])
        XCTAssertEqual(page.countLine, "3 rounds · 1 estimate")
        XCTAssertEqual(
            page.statsTop.map { "\($0.label) \($0.value)" },
            ["Valuation $350B", "Last round Jun 2026", "Min order $10"])
        XCTAssertEqual(
            page.statsBottom.map { "\($0.label) \($0.value)" },
            ["Sell window Tender · quarterly", "Next tender Jan 2027"])
        XCTAssertNil(page.imageURL)
        XCTAssertEqual(PreIpoCopy.privateLine, "Private · priced per round, not live")
        XCTAssertEqual(PreIpoCopy.roundStatsFootnote, "Secondary estimates update between rounds.")
    }

    func testTheStepsRunOldestFirstAndTheEstimateIsLastAndFlagged() throws {
        let page = try profile(detail())

        XCTAssertEqual(
            page.steps.map { "\($0.dateLabel) \($0.priceLabel)" },
            ["Feb ’25 $120", "Oct ’25 $150", "Jun ’26 $165", "Oct 20 est. $185"])
        XCTAssertEqual(page.steps.map(\.isEstimate), [false, false, false, true])
        XCTAssertEqual(page.steps.first?.spokenLabel, "February 2025 round, 120 dollars")
        XCTAssertEqual(page.steps.last?.spokenLabel, "October 20 secondary estimate, 185 dollars")
    }

    func testTheRangesFilterTheRoundsByDateAndTheEstimateAlwaysShows() throws {
        let page = try profile(detail())
        let november2026 = Date(timeIntervalSince1970: 1_793_000_000)

        XCTAssertEqual(page.steps(in: .all, now: november2026).count, 4)
        XCTAssertEqual(
            page.steps(in: .oneYear, now: november2026).map(\.priceLabel), ["$165", "$185"])
        let farFuture = Date(timeIntervalSince1970: 2_300_000_000)
        XCTAssertEqual(page.steps(in: .threeYears, now: farFuture).map(\.priceLabel), ["$185"])
        XCTAssertEqual(page.steps(in: .oneYear, now: farFuture).map(\.isEstimate), [true])
    }

    func testEligibleVersusResearchOnly() throws {
        let eligible = try XCTUnwrap(PreIpoDetail(detail(), locale: locale))
        XCTAssertEqual(eligible.eligibility.title, "Proposal eligible")
        XCTAssertEqual(eligible.eligibility.detail, "your cabals can vote to buy")
        XCTAssertTrue(eligible.eligibility.allowsBuy)

        let research = try XCTUnwrap(PreIpoDetail(detail(tradable: false), locale: locale))
        XCTAssertEqual(research.eligibility.title, "Research only")
        XCTAssertEqual(research.eligibility.detail, "not open to proposals yet")
        XCTAssertFalse(research.eligibility.allowsBuy)
    }

    func testNoNextTenderCellWhenTheCompanyHasNoneScheduled() throws {
        let page = try profile(detail { $0.preIpo?.nextTenderOn = nil })

        XCTAssertEqual(page.statsBottom.map(\.label), ["Sell window"])
    }

    func testAnySellWindowReadsAnyTime() throws {
        let page = try profile(
            detail {
                $0.preIpo?.sellWindow = .anyTime
                $0.preIpo?.nextTenderOn = nil
            })

        XCTAssertEqual(page.statsBottom.map(\.value), ["Any time"])
    }

    @MainActor
    func testAPreIpoDetailWithoutAProfileBuildsTheUnavailableStateNeverListedFacts() async throws {
        let page = try XCTUnwrap(PreIpoDetail(detail(preIpo: false), locale: locale))

        XCTAssertNil(page.profile)
        XCTAssertEqual(page.eligibility, .eligible)
        XCTAssertEqual(PreIpoCopy.estimateUnavailable, "Estimate not available yet")
        XCTAssertEqual(PreIpoCopy.loadFailed, "Couldn’t load this company")

        let presented = AssetDetailClientModel(sampleDetail: detail(preIpo: false), chart: nil)
        XCTAssertEqual(presented.detail?.kind, .preIpo)
        XCTAssertNotNil(presented.detail?.preIpo)
        XCTAssertNil(presented.detail?.preIpo?.profile)
        XCTAssertNil(presented.detail?.stats)
    }

    @MainActor
    func testAListedStockHasNoPreIpoDetail() async {
        XCTAssertNil(PreIpoDetail(.nvda, locale: locale))
        let listed = AssetDetailClientModel(sampleDetail: .nvda, chart: nil)
        XCTAssertNil(listed.detail?.preIpo)
    }

    func testTheRoundsCountAgreesWithItsNumbers() {
        XCTAssertEqual(PreIpoCopy.roundsCount(rounds: 1, estimates: 1), "1 round · 1 estimate")
        XCTAssertEqual(PreIpoCopy.roundsCount(rounds: 0, estimates: 1), "0 rounds · 1 estimate")
    }

    func testEveryPreIpoCopyMemberThatStagingHadStillExists() {
        let strings = [
            PreIpoCopy.sectionTitle, PreIpoCopy.chipLabel, PreIpoCopy.tradesAroundTheClock,
            PreIpoCopy.privateMarketReference, PreIpoCopy.referenceUnavailable, PreIpoCopy.companyValueCaption,
            PreIpoCopy.aboutCardTitle, PreIpoCopy.disclosure, PreIpoCopy.termsLinkTitle, PreIpoCopy.termsURL,
            PreIpoCopy.alsoAvailableFrom, PreIpoCopy.chartEmpty, PreIpoCopy.chartEmptyMessage,
            PreIpoCopy.tokenLabelPlural, PreIpoCopy.tokenLabelSingular, PreIpoCopy.tokensRowLabel,
            PreIpoCopy.referenceCaption(companyValue: "$32.1B", age: "now"), PreIpoCopy.premiumChip(bps: -2700),
            PreIpoCopy.tradingPremiumNudge(bps: 2700),
        ]
        XCTAssertTrue(strings.allSatisfy { !$0.isEmpty })
        XCTAssertTrue(PreIpoCopy.showsPremiumNudge(premiumBps: 1000, assetKind: .preIpo))
        XCTAssertTrue(PreIpoCopy.auditedStrings.contains(PreIpoCopy.disclosure))
    }

    func testTheLedgerLinesOtherTicketsReuse() {
        XCTAssertEqual(
            PreIpoCopy.ledgerLastRound(on: "Jun ’26", price: "$165.00"),
            PreIpoCopy.LedgerLine(label: "Last round · Jun ’26", value: "$165.00"))
        XCTAssertEqual(
            PreIpoCopy.ledgerEstimate(asOf: "Oct 20", price: "$185.00"),
            PreIpoCopy.LedgerLine(label: "Secondary estimate · Oct 20", value: "$185.00"))
    }

    func testAHeldPreIpoRowReadsAtEstimateWithNoReturnWhileAListedRowKeepsItsReturn() throws {
        let rows = Components.Schemas.HeldList.sampleAndres.holdings.map(AssetHeldRowFacts.init)
        let spaceX = try XCTUnwrap(rows.first { $0.title == "AC Capital" && $0.valueText == "$2,220.00" })
        XCTAssertEqual(spaceX.valueNote, "at estimate")
        XCTAssertNil(spaceX.returnText)
        XCTAssertEqual(spaceX.subtitle, "12 sh · 11% of treasury")

        let nvidia = try XCTUnwrap(rows.first { $0.valueText == "$8,360.00" })
        XCTAssertNil(nvidia.valueNote)
        XCTAssertNotNil(nvidia.returnText)
    }
}
