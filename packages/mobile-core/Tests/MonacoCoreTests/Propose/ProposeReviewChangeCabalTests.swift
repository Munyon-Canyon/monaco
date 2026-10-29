import MonacoAPI
import MonacoCore
import XCTest

@MainActor
final class ProposeReviewChangeCabalTests: XCTestCase {
    func testReviewsForTwoCabalsPostToTheirOwnCabalWithTheirOwnKeys() async {
        let service = RecordingService()
        let first = makeModel(service, cabalID: "a")
        let second = makeModel(service, cabalID: "b")

        await first.send()
        await second.send()

        let posts = await service.posts
        XCTAssertEqual(posts.map(\.cabalID), ["a", "b"])
        XCTAssertFalse(posts[0].submission === posts[1].submission)
    }

    private func makeModel(_ service: RecordingService, cabalID: String) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(
                priceComparison: nil, quoteOutAmount: 73_000_000, advisoryCode: nil, advisoryMessage: nil,
                potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: service, cabalID: cabalID, cabal: .init(name: cabalID, voters: "All 3 members"),
            draft: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: "Earnings"), preview: preview,
            trade: .buy(symbol: "GOOGL", kind: .stock, tokenDecimals: 8), buyName: "Alphabet")
    }
}

private actor RecordingService: ProposeService {
    struct Post {
        let cabalID: String
        let submission: IdempotentSubmission
    }
    private(set) var posts: [Post] = []

    func preview(
        cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?
    ) async throws -> ProposePreview {
        throw CancellationError()
    }

    func propose(
        cabalID: String, draft _: ProposalDraft, network _: Components.Schemas.Network?,
        submission: IdempotentSubmission
    ) async throws -> String {
        posts.append(Post(cabalID: cabalID, submission: submission))
        return "proposal-\(cabalID)"
    }

    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {}
}
