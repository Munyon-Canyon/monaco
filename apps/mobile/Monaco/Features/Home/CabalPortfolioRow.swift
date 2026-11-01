import MonacoCore
import SwiftUI

struct CabalPortfolioRow: View {
    let row: HomeCabalRow
    let votes: Int
    let votesText: String?
    var isLast = false

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
            MonacoRow(title: row.name, subtitle: row.subtitle(votesText: votesText), isLast: isLast) {
                lead
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    MoneyText(micros: row.valueMicros, style: .row)
                    if row.isFunded, row.note != .settling { returnText }
                    if let note = row.noteText(now: .now) {
                        Text(note)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .accessibilityIdentifier("cabal-row-\(row.id)-note")
                    }
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-row-\(row.id)")
    }

    @ViewBuilder private var lead: some View {
        let mark = CabalMark(groupId: row.id, name: row.name, size: .row, pictureUrl: row.pictureURL)
            .crestVoteRing(votes > 0)
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.xs) {
                mark
                if let text = CrestMetrics.badgeText(votes) {
                    Text(text)
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.voteNeededLabel)
                        .padding(.horizontal, MonacoTheme.Space.xs)
                        .background(MonacoTheme.voteNeededFill, in: Capsule())
                        .accessibilityHidden(true)
                }
            }
        } else {
            mark.crestBadge(votes)
        }
    }

    @ViewBuilder private var returnText: some View {
        if let bps = row.returnBps {
            PercentText(basisPoints: bps, style: .caption)
        } else {
            PercentText(percentReturn: nil, style: .caption)
        }
    }
}

struct HomeInviteCabalRow: View {
    let row: HomeInviteRow
    var isLast = false
    let open: () -> Void

    var body: some View {
        Button(action: open) {
            MonacoRow(title: row.name, subtitle: row.subtitle, isLast: isLast) {
                CabalMark(
                    groupId: row.invite.cabalID, name: row.name, size: .row, pictureUrl: row.invite.pictureURL)
            } trailing: {
                Text("Invited")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("home-invited-\(row.invite.cabalID)")
    }
}
