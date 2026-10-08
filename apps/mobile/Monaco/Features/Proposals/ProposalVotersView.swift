import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposalVoterGroups: Equatable {
    struct Entry: Equatable, Identifiable {
        let id: String
        let name: String
        let photoURL: URL?
    }

    static let summaryAvatarLimit = 5

    let yes: [Entry]
    let no: [Entry]
    let notVoted: [Entry]

    init(voters: [ProposalVoter], members: [ProposalMember]) {
        let byID = Dictionary(members.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        func entry(_ voter: ProposalVoter) -> Entry {
            let member = byID[voter.id]
            return Entry(id: voter.id, name: member?.name ?? "Member", photoURL: member?.photoURL)
        }
        yes = voters.filter { $0.ballot == "yes" }.map(entry)
        no = voters.filter { $0.ballot == "no" }.map(entry)
        notVoted = voters.filter { $0.ballot != "yes" && $0.ballot != "no" }.map(entry)
    }

    var summaryAvatars: [Entry] { Array((yes + no + notVoted).prefix(Self.summaryAvatarLimit)) }
}

struct ProposalVotersView: View {
    let groups: ProposalVoterGroups

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                group("Yes", groups.yes, vote: "Voted yes")
                group("No", groups.no, vote: "Voted no")
                group("Not voted", groups.notVoted, vote: "Hasn't voted")
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .navigationTitle("Votes")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("proposal-voters")
    }

    @ViewBuilder private func group(_ title: String, _ entries: [ProposalVoterGroups.Entry], vote: String)
        -> some View
    {
        if !entries.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(title, count: nil, trailing: "\(entries.count)")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ForEach(entries) { entry in
                        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: entry.id))) {
                            MonacoRow(
                                title: entry.name, subtitle: vote, isLast: entry.id == entries.last?.id,
                                leading: {
                                    MonacoAvatar(
                                        photoURL: entry.photoURL?.absoluteString, displayName: entry.name,
                                        size: 40, seed: entry.id)
                                },
                                trailing: { EmptyView() })
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("proposal-voter-\(entry.id)")
                    }
                }
            }
        }
    }
}
