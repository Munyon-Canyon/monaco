import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposalVoterGroups: Equatable {
    struct Entry: Equatable, Identifiable {
        let id: String
        let name: String
        let photoURL: URL?
    }

    struct Row: Equatable, Identifiable {
        let entry: Entry
        let ballot: ProposalBallot?
        var id: String { entry.id }
    }

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

    var rows: [Row] {
        yes.map { Row(entry: $0, ballot: .yes) } + no.map { Row(entry: $0, ballot: .no) }
            + notVoted.map { Row(entry: $0, ballot: nil) }
    }

    var totals: String { "\(yes.count) yes, \(no.count) no, \(notVoted.count) not voted" }
}

struct ProposalVotersView: View {
    let model: ProposalDetailModel?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var groups: ProposalVoterGroups {
        ProposalVoterGroups(voters: model?.value?.voters ?? [], members: model?.members ?? [])
    }

    var body: some View {
        content
            .task {
                model?.setVisible(true)
                await model?.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .accessibilityIdentifier("proposal-voters")
    }

    @ViewBuilder private var content: some View {
        if model?.value != nil {
            let groups = groups
            ScrollView {
                MonacoGroupedList {
                    ForEach(groups.rows) { row in
                        voterRow(row, isLast: row.id == groups.rows.last?.id)
                    }
                }
            }
            .safeAreaInset(edge: .top, spacing: 0) { totals(groups) }
        } else if model?.errorMessage != nil {
            failure
        } else {
            loading
        }
    }

    private func totals(_ groups: ProposalVoterGroups) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return VStack(spacing: MonacoTheme.Space.s) {
            layout {
                Text("\(groups.yes.count) yes")
                if !dynamicTypeSize.isAccessibilitySize { Spacer(minLength: MonacoTheme.Space.s) }
                Text("\(groups.no.count) no")
                if !dynamicTypeSize.isAccessibilitySize { Spacer(minLength: MonacoTheme.Space.s) }
                Text("\(groups.notVoted.count) not voted")
            }
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.vertical, MonacoTheme.Space.s)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
                    .fill(MonacoTheme.canvas)
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(groups.totals)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
        }
        .padding(.bottom, MonacoTheme.Space.s)
    }

    private func voterRow(_ row: ProposalVoterGroups.Row, isLast: Bool) -> some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: row.id))) {
            MonacoRow(
                title: row.entry.name, isLast: isLast,
                leading: {
                    MonacoAvatar(
                        photoURL: row.entry.photoURL?.absoluteString, displayName: row.entry.name, size: 36,
                        seed: row.id)
                },
                trailing: { StatusBadge(proposal: .ballot(row.ballot)) })
        }
        .buttonStyle(.monacoRow)
        .accessibilityLabel(ProposalCardCopy.voter(row.entry.name, choice: row.ballot?.rawValue))
        .accessibilityIdentifier("proposal-voter-\(row.id)")
    }

    private var loading: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            SkeletonBlock(height: MonacoSegmentedMetrics.minHeight, radius: MonacoTheme.Radius.chip)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRowSkeleton(rows: 4, markShape: .circle)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading votes")
    }

    private var failure: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            MonacoNotice(.error, message: "Votes couldn't load. Try again.")
            Button("Retry") { Task { await model?.load() } }
                .buttonStyle(.monacoOutline)
                .accessibilityIdentifier("proposal-voters-retry")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
