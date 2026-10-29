import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ProposeReviewModelTests: XCTestCase {
    func testBuyRowsSayShares() {
        let model = makeModel(kind: .stock, service: ReviewService())

        XCTAssertEqual(model.title, "Buy $250.00 of Alphabet")
        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Cabal gets", value: "about 0.73 shares"),
                .init(label: "Price", value: "about $342.47 a share"),
                .init(label: "Pot", value: "50% of $500.00"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
        XCTAssertEqual(model.reason, "Earnings next week.")
    }

    func testBuyHeaderNamesTheOperationAmountAssetAndCabal() {
        let model = makeModel(kind: .stock, service: ReviewService())

        XCTAssertEqual(model.operation, "Buy")
        XCTAssertEqual(model.heroAmount, "$250.00")
        XCTAssertEqual(model.assetName, "GOOGL")
        XCTAssertEqual(model.destination, "to Sunday Investors")
        XCTAssertEqual(model.voteCaption, "All 3 members will vote on this proposal.")
    }

    func testSellHeaderLeadsWithTheQuantity() {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let model = makeSellModel(apple, selling: 60_172_839)

        XCTAssertEqual(model.operation, "Sell")
        XCTAssertEqual(model.heroAmount, "0.6017 shares")
        XCTAssertEqual(model.assetName, "AAPL")
    }

    func testPreIpoRowsSayTokens() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), thesis: "  ")

        XCTAssertEqual(model.rows.first?.value, "about 0.73 tokens")
        XCTAssertEqual(model.rows[1].value, "about $342.47 a token")
        XCTAssertNil(model.reason)
    }

    func testNineDecimalPreIpoBuyCountsOneToken() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), quoteOut: 1_000_000_000, tokenDecimals: 9)

        XCTAssertEqual(model.rows[0], .init(label: "Cabal gets", value: "about 1 token"))
        XCTAssertEqual(model.rows[1], .init(label: "Price", value: "about $250.00 a token"))
    }

    func testUnknownDecimalsLeaveOutCountAndPrice() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), tokenDecimals: nil)

        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Pot", value: "50% of $500.00"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
    }

    func testTheVotingRulesLeadTheRowsAndWhoVotesStaysLast() {
        let ruled = makeModel(
            kind: .stock, service: ReviewService(),
            cabal: .init(
                name: "Sunday Investors", voters: "All 3 members", votingWindow: "1 day", votesToPass: "Majority"))

        XCTAssertEqual(ruled.rows.first, .init(label: "Voting window", value: "1 day"))
        XCTAssertEqual(ruled.rows[1], .init(label: "Votes to pass", value: "Majority"))
        XCTAssertEqual(ruled.rows.last?.label, "Who votes")
        XCTAssertEqual(ruled.operationLine, "Buy Alphabet")
        XCTAssertEqual(ruled.sentMessage, "Buy Alphabet · Sunday Investors")
    }

    func testSendReturnsTheProposalAndNamesTheCabal() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service)

        XCTAssertEqual(model.sendTitle, "Send to vote")
        await model.send()

        XCTAssertEqual(model.proposalID, "proposal-1")
        XCTAssertEqual(model.successToast, "Proposal sent to Sunday Investors")
    }

    func testRetryReplaysTheSameSubmission() async {
        let service = ReviewService(failures: 1)
        let model = makeModel(kind: .stock, service: service)

        await model.send()
        XCTAssertNotNil(model.errorMessage)
        XCTAssertNil(model.proposalID)
        await model.send()

        let submissions = await service.submissions
        XCTAssertEqual(submissions.count, 2)
        XCTAssertTrue(submissions[0] === submissions[1])
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.proposalID, "proposal-1")
    }

    func testSecondSendAfterSuccessMakesNoSecondRequest() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service)

        await model.send()
        await model.send()

        let submissions = await service.submissions
        XCTAssertEqual(submissions.count, 1)
        XCTAssertEqual(model.proposalID, "proposal-1")
    }

    func testReopeningReviewMidSendFindsTheSameSendAndMakesNoSecondRequest() async {
        let service = ReviewService(holdsFirst: true)
        let memory = ProposeReviewMemory()
        let first = memory.hold(makeModel(kind: .stock, service: service))
        let sending = Task { await first.send() }
        await service.waitUntilHeld()

        let reopened = memory.hold(makeModel(kind: .stock, service: service))
        XCTAssertTrue(reopened === first)
        XCTAssertTrue(reopened.isSending)
        await reopened.send()
        await service.release()
        await sending.value

        let submissions = await service.submissions
        XCTAssertEqual(submissions.count, 1)
        XCTAssertEqual(first.proposalID, "proposal-1")
        XCTAssertEqual(memory.model(for: first.draft)?.proposalID, "proposal-1")
    }

    func testChangedAmountGetsANewModelAndSubmission() async {
        let service = ReviewService()
        let memory = ProposeReviewMemory()
        let first = memory.hold(makeModel(kind: .stock, service: service, amount: 250_000_000))
        let other = makeModel(kind: .stock, service: service, amount: 300_000_000)

        XCTAssertNil(memory.model(for: other.draft))
        let second = memory.hold(other)
        XCTAssertTrue(second === other)
        XCTAssertNil(memory.model(for: first.draft))
        await first.send()
        await second.send()

        let submissions = await service.submissions
        XCTAssertEqual(submissions.count, 2)
        XCTAssertFalse(submissions[0] === submissions[1])
    }

    func testChangedReasonGetsANewModel() {
        let memory = ProposeReviewMemory()
        let first = memory.hold(makeModel(kind: .stock, service: ReviewService(), thesis: "Earnings next week."))
        let other = makeModel(kind: .stock, service: ReviewService(), thesis: "Cloud is growing.")

        XCTAssertNil(memory.model(for: other.draft))
        XCTAssertFalse(memory.hold(other) === first)
    }

    func testSellSaysSharesRaisesAndWhatTheCabalKeeps() {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let model = makeSellModel(apple, selling: 60_172_839)

        XCTAssertEqual(model.title, "Sell 0.6017 shares of AAPL")
        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Raises", value: "about $139.00"),
                .init(label: "Cabal keeps", value: "0.6017 shares"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
        XCTAssertEqual(model.reasonTitle, "Thesis")
    }

    func testPreIpoSellSaysTokens() {
        let spacex = ProposeHoldingTests.holding(
            name: "SpaceX", kind: .preIpo, units: "2.0000", tokenAmount: 2_000_000_000)
        let model = makeSellModel(spacex, selling: 2_000_000_000)

        XCTAssertEqual(model.title, "Sell 2 tokens of SPACEX")
        XCTAssertEqual(
            model.rows,
            [.init(label: "Raises", value: "about $139.00"), .init(label: "Who votes", value: "All 3 members")])
    }

    func testTinyBuyOfALargePotSaysLessThanOnePercent() {
        let model = makeModel(kind: .stock, service: ReviewService(), amount: 2_000_000, pot: 5_000_000_000)

        XCTAssertEqual(model.title, "Buy $2.00 of Alphabet")
        XCTAssertTrue(model.rows.contains(.init(label: "Pot", value: "Less than 1% of $5,000.00")))
        XCTAssertFalse(model.rows.contains { $0.value.hasPrefix("0%") })
    }

    func testSendWithTheDefaultChainLeavesTheNetworkOut() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, comparison: BestPrice.sampleComparison(now: Date()))

        await model.send()

        let networks = await service.networks
        XCTAssertEqual(model.chain, .best)
        XCTAssertEqual(networks, [nil])
    }

    func testAChosenChainSetsTheRowsAndTheSentNetwork() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, comparison: BestPrice.sampleComparison(now: Date()))

        model.choose(.network(.ethereum))
        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Cabal gets", value: "about 0.0398 shares"),
                .init(label: "Price", value: "about $251.26 a share"),
                .init(label: "Pot", value: "50% of $500.00"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
        await model.send()

        let networks = await service.networks
        XCTAssertEqual(networks, [.ethereum])
    }

    func testChoosingAfterASuccessfulSendChangesNothing() async {
        let model = makeModel(
            kind: .stock, service: ReviewService(), comparison: BestPrice.sampleComparison(now: Date()))
        let rows = model.rows

        await model.send()
        model.choose(.network(.solana))

        XCTAssertEqual(model.chain, .best)
        XCTAssertEqual(model.rows, rows)
    }

    private func makeSellModel(_ holding: ProposeHolding, selling amount: Int64) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: 139_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: ReviewService(), cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: holding.symbol, tokenAmount: amount, thesis: ""), preview: preview,
            trade: .sell(holding))
    }

    private func makeModel(
        kind: AssetKind, service: ReviewService, thesis: String = "Earnings next week.",
        quoteOut: Int64 = 73_000_000, tokenDecimals: Int? = 8, amount: Int64 = 250_000_000,
        pot: Int64 = 500_000_000, comparison: Components.Schemas.PriceComparison? = nil,
        cabal: ProposeCabalInfo = .init(name: "Sunday Investors", voters: "All 3 members")
    ) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(
                priceComparison: comparison, quoteOutAmount: quoteOut, advisoryCode: nil, advisoryMessage: nil,
                potValueMicros: pot))
        return ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: cabal,
            draft: .buy(symbol: "GOOGL", usdcMicros: amount, thesis: thesis), preview: preview,
            trade: .buy(symbol: "GOOGL", kind: kind, tokenDecimals: tokenDecimals), buyName: "Alphabet")
    }
}

actor ReviewService: ProposeService {
    struct Failure: Error {}
    private var failures: Int
    private var holdsFirst: Bool
    private var held: CheckedContinuation<Void, Never>?
    private(set) var submissions: [IdempotentSubmission] = []
    private(set) var networks: [Components.Schemas.Network?] = []
    private(set) var previewNetworks: [Components.Schemas.Network?] = []
    private let chainQuoteOut: Int64
    private let chainCode: String?
    private var holdsChain: Bool
    private var heldChain: CheckedContinuation<Void, Never>?

    init(
        failures: Int = 0, holdsFirst: Bool = false, chainQuoteOut: Int64 = 99_000_000, chainCode: String? = nil,
        holdsChain: Bool = false
    ) {
        self.failures = failures
        self.holdsFirst = holdsFirst
        self.chainQuoteOut = chainQuoteOut
        self.chainCode = chainCode
        self.holdsChain = holdsChain
    }

    func waitUntilChainAsked() async -> Bool {
        while heldChain == nil { await Task.yield() }
        return true
    }

    func releaseChain() {
        holdsChain = false
        heldChain?.resume()
        heldChain = nil
    }

    func waitUntilHeld() async {
        while held == nil { await Task.yield() }
    }

    func release() {
        held?.resume()
        held = nil
    }

    func preview(
        cabalID _: String, draft _: ProposalDraft, network: Components.Schemas.Network?
    ) async throws -> ProposePreview {
        previewNetworks.append(network)
        if holdsChain { await withCheckedContinuation { heldChain = $0 } }
        return ProposePreview(
            .init(
                quoteOutAmount: chainQuoteOut, advisoryCode: chainCode, advisoryMessage: nil,
                potValueMicros: 500_000_000))
    }

    func propose(
        cabalID _: String, draft _: ProposalDraft, network: Components.Schemas.Network?,
        submission: IdempotentSubmission
    ) async throws -> String {
        submissions.append(submission)
        networks.append(network)
        if holdsFirst {
            holdsFirst = false
            await withCheckedContinuation { held = $0 }
        }
        if failures > 0 {
            failures -= 1
            throw Failure()
        }
        return "proposal-1"
    }

    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {}
}
