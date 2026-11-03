import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

#if DEBUG
@MainActor
struct ProposalSampleRepliesTests {
    @Test func theOpenProposalQuotesLoganAndFoldsOneReplyUnderKai() throws {
        let script = ProposalSampleScenario.nvdaOpen.script

        let threads = try #require(script.comments)
        #expect(threads[0].comment.author.displayName == "Sam")
        #expect(threads[0].comment.replyTo?.authorDisplayName == "Logan")
        #expect(threads[1].comment.author.displayName == "Kai")
        #expect(threads[1].replies.count == 1)
        #expect(script.commentsHaveMore)
        #expect(script.proposal?.replyCount == 6)
    }

    @Test func everyClosedFrameLeadsWithTheMonacoReply() throws {
        let scenarios: [(ProposalSampleScenario, Int)] = [
            (.nvdaPassed, 7), (.nvdaFilled, 7), (.closedSidesOn, 7), (.tslaFailed, 4),
        ]

        for (scenario, count) in scenarios {
            let script = scenario.script
            let first = try #require(script.comments?.first)
            #expect(first.comment.author.kind == .system, "\(scenario)")
            #expect(first.comment.author.displayName == "Monaco", "\(scenario)")
            #expect(script.proposal?.replyCount == count, "\(scenario)")
        }
    }

    @Test func thePreIpoProposalShowsNorasReplyWithItsQuoteAndHeart() throws {
        let script = ProposalSampleScenario.spaceXOpen.script

        let reply = try #require(script.comments?.first)
        #expect(reply.comment.author.displayName == "Nora")
        #expect(reply.comment.replyTo?.authorDisplayName == "Logan")
        #expect(reply.comment.reactions.first?.count == 2)
        #expect(script.proposal?.replyCount == 3)
    }

    @Test func aProposalWithNoScriptedRepliesKeepsTheDefaultThread() {
        #expect(ProposalSampleScenario.open.script.comments == nil)
    }

    @Test func eachSampleGifDrawsItsOwnPhoto() {
        let ids = SampleGifProvider.items.map(\.id)
        let scenes = Set(ids.map { GifSampleStill.scene(for: $0).imageName })

        #expect(ids.count == 8)
        #expect(scenes.count == ids.count)
    }
}
#endif
