import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalLiveFiguresTests: XCTestCase {
    func testTheDetailCarriesTheCabalNameToItsSummary() throws {
        let detail = ProposalDetail(.sample())
        XCTAssertEqual(detail.summary.cabalName, Components.Schemas.Cabal.sample(role: nil).name)
    }

    func testAConfirmedBuyShowsTheExecutedPriceAndUnitsFilled() {
        let swap = Self.swap(.confirmed, usdc: 25_000_000, tokens: 12_500_000, price: 200_000_000)
        XCTAssertEqual(
            swap.executionFigures(kind: .stock), ProposalExecutionFigures(price: "$200.00", units: "0.125 shares"))
    }

    func testAConfirmedSellShowsTheFiguresTheSameWay() {
        let swap = Self.swap(.confirmed, usdc: 440_000_000, tokens: 200_000_000, price: 220_000_000)
        XCTAssertEqual(
            swap.executionFigures(kind: .stock), ProposalExecutionFigures(price: "$220.00", units: "2 shares"))
    }

    func testAPreIpoSwapNamesItsUnitsAsTokens() {
        let swap = Self.swap(.confirmed, usdc: 25_000_000, tokens: 250_000_000, price: 10_000_000)
        XCTAssertEqual(swap.executionFigures(kind: .preIpo)?.units, "2.5 tokens")
    }

    func testASwapThatHasNotConfirmedHasNoExecutionFigures() {
        for status in [Components.Schemas.ProposalDetail.SwapPayload.StatusPayload.created, .submitted, .failed] {
            let swap = Self.swap(status, usdc: 25_000_000, tokens: nil, price: nil)
            XCTAssertNil(swap.executionFigures(kind: .stock), "\(status)")
        }
    }

    func testAConfirmedSwapMissingItsPriceHasNoExecutionFigures() {
        let unpriced = Self.swap(.confirmed, usdc: 25_000_000, tokens: 12_500_000, price: nil)
        XCTAssertNil(unpriced.executionFigures(kind: .stock))
        XCTAssertNil(ProposalSwap(nil).executionFigures(kind: .stock))
    }

    func testTheSwapMapsEveryFigureTheServerSends() {
        let swap = Self.swap(.confirmed, usdc: 25_000_000, tokens: 12_500_000, price: 200_000_000)
        XCTAssertEqual(swap.usdcMicros, 25_000_000)
        XCTAssertEqual(swap.tokenAmount, 12_500_000)
        XCTAssertEqual(swap.tokenDecimals, 8)
        XCTAssertEqual(swap.priceMicros, 200_000_000)
    }

    func testAProposerWhoSharesShowsTheirOneMonthReturn() {
        assertReturn(ProposerReturn(bps: 820, isPrivate: false), "+8.2% · 1M", .gain)
        assertReturn(ProposerReturn(bps: -350, isPrivate: false), "\u{2212}3.5% · 1M", .loss)
    }

    func testAReturnThatRoundsToZeroIsMuted() {
        assertReturn(ProposerReturn(bps: 0, isPrivate: false), "0.0% · 1M", .muted)
        assertReturn(ProposerReturn(bps: 3, isPrivate: false), "0.0% · 1M", .muted)
    }

    func testAProposerWhoHidesTheirReturnShowsPnLPrivate() {
        assertReturn(ProposerReturn(bps: nil, isPrivate: true), "P&L private", .muted)
        assertReturn(ProposerReturn(bps: 820, isPrivate: true), "P&L private", .muted)
    }

    func testAnUnrankedProposerShowsNoReturn() {
        XCTAssertNil(ProposerReturn(bps: nil, isPrivate: false))
    }

    func testTheDetailMapsTheProposersReturnFromItsTwoFields() {
        var detail = Components.Schemas.ProposalDetail.sample()
        assertReturn(ProposalDetail(detail).proposerReturn, "+8.2% · 1M", .gain)
        detail.proposerReturnPrivate = true
        detail.proposerReturn1mBps = nil
        assertReturn(ProposalDetail(detail).proposerReturn, "P&L private", .muted)
        detail.proposerReturnPrivate = false
        XCTAssertNil(ProposalDetail(detail).proposerReturn)
    }

    func testTheDetailMapsTheLinkedSwapFigures() {
        var detail = Components.Schemas.ProposalDetail.sample(status: .executed)
        detail.swap = Self.payload(.confirmed, usdc: 25_000_000, tokens: 12_500_000, price: 200_000_000)
        let swap = ProposalDetail(detail).summary.swap
        XCTAssertEqual(swap?.priceMicros, 200_000_000)
        XCTAssertEqual(swap?.executionFigures(kind: .stock)?.price, "$200.00")
    }

    private func assertReturn(
        _ value: ProposerReturn?, _ text: String, _ tone: ProposerReturn.Tone, line: UInt = #line
    ) {
        XCTAssertEqual(value?.text, text, line: line)
        XCTAssertEqual(value?.tone, tone, line: line)
    }

    private static func swap(
        _ status: Components.Schemas.ProposalDetail.SwapPayload.StatusPayload, usdc: Int64?, tokens: Int64?,
        price: Int64?
    ) -> ProposalSwap {
        ProposalSwap(payload(status, usdc: usdc, tokens: tokens, price: price))
    }

    private static func payload(
        _ status: Components.Schemas.ProposalDetail.SwapPayload.StatusPayload, usdc: Int64?, tokens: Int64?,
        price: Int64?
    ) -> Components.Schemas.ProposalDetail.SwapPayload {
        .init(
            swapId: "swap-1", status: status, failureCode: nil, failureMessage: nil, txSignature: nil,
            retryable: false, usdcMicros: usdc, tokenAmount: tokens, tokenDecimals: 8, priceMicros: price)
    }
}
