import MonacoCore
import SwiftUI

struct ProposalCardView: View {
    let card: ProposalCard
    let isCasting: Bool
    let vote: (BallotChoice) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            NavigationLink(value: AnyAppRoute(ProposalRoute(proposalID: card.id))) {
                TimelineView(.everyMinute) { context in
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                        ProposalCardHeader(card: card, now: context.date)
                        let age = ProposalCardCopy.age(since: card.proposal.createdAt, now: context.date)
                        ProposalByline(person: card.proposer, text: "\(card.proposer.name) · \(age)")
                        if let thesis = card.proposal.thesis {
                            Text(thesis)
                                .font(MonacoTheme.Typo.callout)
                                .foregroundStyle(MonacoTheme.muted)
                                .lineLimit(3)
                                .fixedSize(horizontal: false, vertical: true)
                                .accessibilityIdentifier("proposal-card-reason")
                        }
                        ProposalTracker(card: card)
                    }
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("proposal-card-open-\(card.id)")
            ProposalBallotRow(ballot: card.ballot, isCasting: isCasting, vote: vote)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous).fill(MonacoTheme.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-card-\(card.id)")
    }
}

struct ProposalCardHeader: View {
    let card: ProposalCard
    let now: Date

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    identity
                    corner
                }
            } else {
                HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
                    identity
                    Spacer(minLength: MonacoTheme.Space.s)
                    corner
                }
            }
            Text(card.amount)
                .moneyFont(.large)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("proposal-card-amount")
        }
    }

    private var identity: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: card.asset.symbol, displayName: card.asset.name, assetKind: card.asset.kind, size: 40,
                logoURL: card.asset.logoURL)
            VStack(alignment: .leading, spacing: 2) {
                Text(card.asset.ticker)
                    .font(MonacoTheme.Typo.ticker)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                Text(card.proposal.kind.title)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var corner: some View {
        if let chip = card.chip {
            ProposalStatusChip(label: chip, status: card.proposal.status, swap: card.swap)
        } else if let closes = card.closes(now: now) {
            Text(closes)
                .font(MonacoTheme.Typo.stamp)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .lineLimit(1)
                .accessibilityIdentifier("proposal-card-closes")
        }
    }
}

struct ProposalByline: View {
    let person: ProposalPerson
    let text: String

    var body: some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: person.userID))) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(photoURL: person.photoURL, displayName: person.name, size: 28, seed: person.userID)
                Text(text)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("proposal-proposer")
    }
}

struct ProposalTracker: View {
    static let maxDots = 12

    let card: ProposalCard

    var body: some View {
        let tally = card.proposal.tally
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if tally.voters <= Self.maxDots {
                HStack(spacing: 6) {
                    ForEach(0..<tally.voters, id: \.self) { index in
                        Circle()
                            .fill(index < tally.voted ? MonacoTheme.brandFill : Color.clear)
                            .overlay(Circle().strokeBorder(MonacoTheme.brandFill, lineWidth: 1.5))
                            .frame(width: 10, height: 10)
                    }
                }
                .accessibilityHidden(true)
            }
            Text(card.tracker)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("proposal-tracker")
        }
    }
}

struct ProposalBallotRow: View {
    let ballot: ProposalBallot
    let isCasting: Bool
    let vote: (BallotChoice) -> Void

    @State private var isChanging = false

    var body: some View {
        switch ballot {
        case .none:
            EmptyView()
        case .ask:
            buttons
        case .voted(let choice):
            if isChanging {
                buttons
            } else {
                HStack(spacing: MonacoTheme.Space.s) {
                    Text(ProposalCardCopy.viewerVote(choice))
                        .font(MonacoTheme.Typo.calloutStrong)
                        .foregroundStyle(MonacoTheme.ink)
                        .accessibilityIdentifier("proposal-voted")
                    Spacer(minLength: MonacoTheme.Space.s)
                    Button("Change") { isChanging = true }
                        .font(MonacoTheme.Typo.calloutStrong)
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minHeight: 44)
                        .accessibilityIdentifier("proposal-vote-change")
                }
            }
        }
    }

    private var buttons: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Button("Yes") { cast(.yes) }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("proposal-vote-yes")
            Button("No") { cast(.no) }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("proposal-vote-no")
        }
        .monacoFullWidthButtons()
        .disabled(isCasting)
        .opacity(isCasting ? 0.6 : 1)
    }

    private func cast(_ choice: BallotChoice) {
        Haptics.tap()
        isChanging = false
        vote(choice)
    }
}

struct ProposalCardSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 40, height: 40, radius: 20)
                VStack(alignment: .leading, spacing: 6) {
                    SkeletonBlock(width: 64, height: 14)
                    SkeletonBlock(width: 36, height: 10)
                }
                Spacer()
                SkeletonBlock(width: 80, height: 12)
            }
            SkeletonBlock(width: 120, height: 28)
            SkeletonBlock(width: 160, height: 14)
            SkeletonBlock(height: 12)
            SkeletonBlock(width: 140, height: 10)
            HStack(spacing: MonacoTheme.Space.s) {
                SkeletonBlock(height: 44, radius: 22)
                SkeletonBlock(height: 44, radius: 22)
            }
        }
        .padding(MonacoTheme.Space.m)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous).fill(MonacoTheme.surface)
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading")
        .accessibilityIdentifier("proposal-card-skeleton")
    }
}

struct ProposalLoadFailedRow: View {
    let message: String
    let retry: () -> Void

    var body: some View {
        HStack {
            Text(message)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
            Spacer()
            Button("Try again", action: retry)
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("proposal-retry")
        }
        .accessibilityIdentifier("proposal-load-failed")
    }
}
