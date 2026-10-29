import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposalVotersView: View {
    let model: ProposalDetailModel?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @State private var voters: ProposalVotersPager?

    private var sidesVisible: Bool { model?.value?.sidesVisible ?? false }

    private func makeSides(_ voters: ProposalVotersPager) -> ProposalSides {
        ProposalSides.make(
            voters: voters.rows.map(\.voter), members: model?.members ?? [],
            viewerID: environment?.viewer?.userID ?? "", sidesVisible: sidesVisible)
    }

    var body: some View {
        content
            .task {
                model?.setVisible(true)
                await model?.observe()
            }
            .task(id: model?.value?.summary.tally) {
                guard let summary = model?.value?.summary, let environment else { return }
                let pager = voters ?? ProposalVotersPager(proposalID: summary.id, api: environment.api)
                voters = pager
                await pager.loadFirst()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .accessibilityIdentifier("proposal-voters")
    }

    @ViewBuilder private var content: some View {
        if let tally = model?.value?.summary.tally {
            if let voters, !voters.rows.isEmpty {
                let sides = makeSides(voters)
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(sides.rows) { row in
                            voterRow(row, isLast: row.id == sides.rows.last?.id && voters.phase == .exhausted)
                        }
                        pagingFooter(voters)
                    }
                    .frame(maxWidth: .infinity)
                }
                .safeAreaInset(edge: .top, spacing: 0) { totals(ProposalVoterTotals(tally)) }
            } else if let voters, case .failed = voters.phase {
                failure
            } else {
                loading
            }
        } else if model?.errorMessage != nil {
            failure
        } else {
            loading
        }
    }

    @ViewBuilder private func pagingFooter(_ voters: ProposalVotersPager) -> some View {
        switch voters.phase {
        case .idle:
            Color.clear.frame(height: 1).onAppear { Task { await voters.loadMore() } }
        case .loadingMore:
            MonacoRowSkeleton(rows: 1, markShape: .circle)
        case .failed:
            MonacoErrorRow(thing: "more votes", identifier: "proposal-voters-more-failed") {
                Task { await voters.loadMore() }
            }
        default:
            EmptyView()
        }
    }

    private func totals(_ totals: ProposalVoterTotals) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        let counts = totals.counts(sidesVisible: sidesVisible)
        return VStack(spacing: MonacoTheme.Space.s) {
            layout {
                ForEach(Array(counts.enumerated()), id: \.offset) { index, text in
                    if index > 0, !dynamicTypeSize.isAccessibilitySize { Spacer(minLength: MonacoTheme.Space.s) }
                    Text(text)
                }
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
            .accessibilityLabel(totals.strip(sidesVisible: sidesVisible))
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if !sidesVisible {
                Text(privacyCaption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("proposal-voters-private")
            }
            MonacoRule()
        }
        .padding(.bottom, MonacoTheme.Space.s)
    }

    private var privacyCaption: String {
        let isOpen = model?.summary?.state(now: .now).isOpen == true
        return "Who voted which way stays private in this cabal." + (isOpen ? " Counts are live." : "")
    }

    private func badge(_ side: ProposalSides.Side) -> StatusBadge {
        switch side {
        case .yes: StatusBadge("Yes", tone: .positive)
        case .no: StatusBadge("No", tone: .negative)
        case .voted: StatusBadge("Voted", tone: .neutral)
        case .notVoted: StatusBadge("Not voted", tone: .neutral)
        }
    }

    private func spoken(_ row: ProposalSides.Row) -> String {
        switch row.side {
        case .yes: ProposalCardCopy.voter(row.entry.name, choice: "yes")
        case .no: ProposalCardCopy.voter(row.entry.name, choice: "no")
        case .voted: "\(row.entry.name) voted"
        case .notVoted: ProposalCardCopy.voter(row.entry.name, choice: nil)
        }
    }

    private func voterRow(_ row: ProposalSides.Row, isLast: Bool) -> some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: row.id))) {
            MonacoRow(
                title: row.entry.name, isLast: isLast,
                leading: {
                    MonacoAvatar(
                        photoURL: row.entry.photoURL?.absoluteString, displayName: row.entry.name, size: 36,
                        seed: row.id)
                },
                trailing: { badge(row.side) })
        }
        .buttonStyle(.monacoRow)
        .accessibilityLabel(spoken(row))
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
            Button("Retry") {
                Task {
                    await model?.load()
                    await voters?.loadFirst()
                }
            }
            .buttonStyle(.monacoOutline)
            .accessibilityIdentifier("proposal-voters-retry")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
