import MonacoCore
import SwiftUI

struct FeedProposalAttachment: View {
    let tally: FeedPostFacts.Tally
    let identifier: String
    var line: String?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            VoteTallyBar(yes: tally.yes, no: tally.no, eligible: tally.voters)
            VoteTallyEnds(
                yes: tally.yes, no: tally.no, middle: "", identifierPrefix: "feed", identifierSuffix: identifier)
            VoteStatusLine(
                result: nil, facts: [line ?? tally.statusLine], identifierPrefix: "feed", identifierSuffix: identifier,
                minHeight: 0)
        }
        .padding(.vertical, MonacoTheme.Space.xs)
    }
}
