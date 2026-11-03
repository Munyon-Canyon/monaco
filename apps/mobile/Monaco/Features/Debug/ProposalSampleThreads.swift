#if DEBUG
import Foundation
import MonacoAPI

enum ProposalSampleThreads {
    typealias Thread = Components.Schemas.CommentThread

    private static let heart: [Components.Schemas.Reaction] = [.init(emoji: "❤️", count: 1, mine: false)]

    static let open: [Thread] = [
        Thread(
            comment: .sample(
                "f-sam", author: "Sam", body: "Size is fine. Yes from me.", minutesAgo: 3,
                quote: .sample("f-logan", author: "Logan", excerpt: "44% NVDA after this."), reactions: heart),
            replies: []),
        Thread(
            comment: .sample("f-kai", author: "Kai", body: "No for now, I would wait for the print.", minutesAgo: 1),
            replies: [
                .sample("f-priya", author: "Priya", parent: "f-kai", body: "Same. Waiting too.", minutesAgo: 1)
            ]),
    ]

    static let passed: [Thread] = [
        Thread.nvdaPassedMonacoReply,
        Thread(comment: .sample("f-mia", author: "Mia", body: "Filled under our price.", minutesAgo: 10), replies: []),
    ]

    static let failed: [Thread] = [
        Thread.tslaFailedMonacoReply,
        Thread(
            comment: .sample("f-priya-late", author: "Priya", body: "I missed this one.", minutesAgo: 60), replies: []),
    ]

    static let preIpo: [Thread] = [
        Thread(
            comment: .sample(
                "f-nora", author: "Nora", body: "12% over the round is fine for a starter.", minutesAgo: 60,
                quote: .sample("f-logan-ipo", author: "Logan", excerpt: "Secondary trades sit near $185."),
                reactions: [.init(emoji: "❤️", count: 2, mine: false)]),
            replies: [])
    ]
}

extension Components.Schemas.ProposalDetail {
    fileprivate func withReplies(_ count: Int) -> Self {
        var detail = self
        detail.replyCount = count
        return detail
    }
}

extension SampleAPIScript {
    mutating func showReplies(_ threads: [Components.Schemas.CommentThread], total: Int, hasMore: Bool = true) {
        comments = threads
        commentsHaveMore = hasMore
        proposal = proposal?.withReplies(total)
    }
}
#endif
