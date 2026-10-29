import MonacoCore
import SwiftUI

struct CabalPortfolioRow: View {
    let row: HomeCabalRow
    let votes: Int
    let votesText: String?
    var isLast = false

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
            MonacoRow(title: row.name, subtitle: row.subtitle(votesText: votesText), isLast: isLast) {
                CabalMark(groupId: row.id, name: row.name, size: .row, pictureUrl: row.pictureURL)
                    .crestVoteRing(votes > 0)
                    .crestBadge(votes)
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    MoneyText(micros: row.valueMicros, style: .row)
                    if row.isFunded { returnText }
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-row-\(row.id)")
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
