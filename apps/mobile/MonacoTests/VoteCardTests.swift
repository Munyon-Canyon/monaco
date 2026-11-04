import Foundation
import MonacoCore
import Testing
import UIKit

@testable import Monaco

@MainActor
struct VoteCardTests {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private var closes: Date { now.addingTimeInterval(4 * 3600) }
    private let tally = ProposalTally(yes: 4, no: 1, voters: 8, needed: 5)
    private let dot = " \u{00B7} "

    private func line(cabal: String?, replies: Int?) -> VoteCardLine {
        ProposalVoteCopy.cardLine(cabalName: cabal, tally: tally, replies: replies, expiresAt: closes, now: now)
    }

    @Test func theCabalLineDropsClosesInForTheShortForm() {
        let line = line(cabal: "AC Capital", replies: 6)
        #expect(line.short == ["AC Capital", "5 of 8 voted", "6 replies", "4h"].joined(separator: dot))
        #expect(line.full == ["AC Capital", "5 of 8 voted", "6 replies", "closes in 4h"].joined(separator: dot))
    }

    @Test func theCabalessLineSpellsOutClosesIn() {
        let line = line(cabal: nil, replies: 6)
        #expect(line.full == ["5 of 8 voted", "6 replies", "closes in 4h"].joined(separator: dot))
    }

    @Test func theFeedLineLeavesTheRepliesOut() {
        let line = line(cabal: "AC Capital", replies: nil)
        #expect(line.short == ["AC Capital", "5 of 8 voted", "4h"].joined(separator: dot))
    }

    private func textWidth(_ text: String) -> CGFloat {
        let font = UIFont.preferredFont(
            forTextStyle: .footnote, compatibleWith: UITraitCollection(preferredContentSizeCategory: .large))
        return (text as NSString).size(withAttributes: [.font: font]).width
    }

    @Test func theCabalessFullLineFitsAt393AndTheShortFormTakesOverAt320() {
        let line = line(cabal: nil, replies: 6)
        #expect(textWidth(line.full) <= VoteCard<EmptyView>.lineWidth(screenWidth: 393))
        #expect(textWidth(line.full) > VoteCard<EmptyView>.lineWidth(screenWidth: 320))
        #expect(textWidth(line.short) <= VoteCard<EmptyView>.lineWidth(screenWidth: 320))
    }

    @Test func theCabalLineNeedsTheShortFormAt393() {
        let line = line(cabal: "AC Capital", replies: 6)
        #expect(textWidth(line.full) > VoteCard<EmptyView>.lineWidth(screenWidth: 393))
        #expect(textWidth(line.short) <= VoteCard<EmptyView>.lineWidth(screenWidth: 393))
    }
}
