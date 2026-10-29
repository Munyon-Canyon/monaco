import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

struct ProposalVoterGroupsTests {
    @Test func fiftyVotersGroupByBallotAndTheRowsListYesThenNoThenNotVoted() {
        let voters = (0..<50).map { index in
            switch index % 3 {
            case 0: voter(index, choice: .yes)
            case 1: voter(index, choice: .no)
            default: voter(index, choice: nil)
            }
        }
        let groups = ProposalVoterGroups(voters: voters, members: [])
        #expect(groups.yes.count + groups.no.count + groups.notVoted.count == 50)
        #expect(groups.yes.count == 17)
        #expect(groups.notVoted.count == 16)
        #expect(groups.rows.count == 50)
        #expect(groups.rows.prefix(17).allSatisfy { $0.ballot == .yes })
        #expect(groups.rows.dropFirst(17).prefix(17).allSatisfy { $0.ballot == .no })
        #expect(groups.rows.suffix(16).allSatisfy { $0.ballot == nil })
    }

    private func voter(_ index: Int, choice: Components.Schemas.ProposalVoter.ChoicePayload?) -> ProposalVoter {
        ProposalVoter(
            .init(
                userId: "u\(index)", voted: choice != nil, choice: choice, castAt: choice == nil ? nil : .now,
                displayName: "Member", photoUrl: nil))
    }
}
