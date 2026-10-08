import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

struct ProposalVoterGroupsTests {
    @Test func fiftyVotersGroupByBallotAndTheSummaryDrawsFive() {
        let voters = (0..<50).map { index in
            switch index % 3 {
            case 0: ProposalVoter(.init(userId: "u\(index)", choice: .yes, castAt: .now))
            case 1: ProposalVoter(.init(userId: "u\(index)", choice: .no, castAt: .now))
            default: ProposalVoter(.init(userId: "u\(index)", choice: nil, castAt: nil))
            }
        }
        let groups = ProposalVoterGroups(voters: voters, members: [])
        #expect(groups.yes.count + groups.no.count + groups.notVoted.count == 50)
        #expect(groups.yes.count == 17)
        #expect(groups.notVoted.count == 16)
        #expect(groups.summaryAvatars.count == 5)
    }
}
