import MonacoCore
import SwiftUI

struct FeedProposalAttachment: View {
    let tally: FeedPostFacts.Tally
    let identifier: String

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VoteTallyBar(yes: tally.yes, no: tally.no, eligible: tally.voters)
            VoteTallyEnds(
                yes: tally.yes, no: tally.no, middle: "", identifierPrefix: "feed", identifierSuffix: identifier)
            VoteStatusLine(
                result: nil, facts: [tally.statusLine], identifierPrefix: "feed", identifierSuffix: identifier)
        }
        .padding(.vertical, MonacoTheme.Space.xs)
    }
}
