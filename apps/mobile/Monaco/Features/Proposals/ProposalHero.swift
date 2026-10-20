import MonacoCore
import SwiftUI

extension ProposerReturn.Tone {
    fileprivate var pill: PnLPill.Tone {
        switch self {
        case .gain: .gain
        case .loss: .loss
        case .muted: .muted
        }
    }
}

struct ProposalHero: View {
    let summary: ProposalSummary
    let asset: ProposalAsset?
    let members: [ProposalMember]
    let paused: Bool
    var authorReturn: ProposerReturn?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var state: ProposalState { summary.state(now: .now) }
    private var proposer: ProposalMember? { members.first { $0.id == summary.proposerID } }
    private var cabalLabel: String { summary.cabalName.isEmpty ? "Your cabal" : summary.cabalName }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                StockMark(
                    symbol: summary.symbol, displayName: asset?.displayName, assetKind: asset?.kind ?? .stock,
                    size: .review, logoURL: asset?.logoURL)
                Spacer(minLength: MonacoTheme.Space.s)
                StatusBadge(proposal: .hero(state))
                    .accessibilityIdentifier("proposal-status-chip")
            }
            Text(summary.title(asset: asset))
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, MonacoTheme.Space.m)
            amount
                .padding(.top, MonacoTheme.Space.s)
            author
                .padding(.top, MonacoTheme.Space.m)
            Text("\(cabalLabel) · \(ProposalCardCopy.ago(since: summary.createdAt, now: .now))")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.top, MonacoTheme.Space.xs)
            if paused && state.isOpen {
                MonacoNotice(.info, message: ProposalCardCopy.pausedCaption(isSell: summary.kind == "sell"))
                    .padding(.top, MonacoTheme.Space.m)
                    .accessibilityIdentifier("proposal-paused-caption")
            }
        }
    }

    @ViewBuilder private var amount: some View {
        if summary.kind == "sell" {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(ProposalCard.shareLabel(summary: summary, asset: asset))
                    .moneyFont(.large)
                    .foregroundStyle(MonacoTheme.ink)
                if summary.quoteOutAmount > 0 {
                    Text("About \(UsdAmountFormatter.format(micros: summary.quoteOutAmount))")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                }
            }
        } else if let micros = summary.usdcMicros {
            MoneyText(micros: micros, style: .large)
        }
    }

    private var author: some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout =
            stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: summary.proposerID))) {
            layout {
                HStack(spacing: MonacoTheme.Space.s) {
                    MonacoAvatar(
                        photoURL: proposer?.photoURL?.absoluteString, displayName: proposer?.name ?? "Member",
                        size: 24, seed: summary.proposerID)
                    Text(proposer?.name ?? "Member")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.ink)
                        .lineLimit(stacked ? nil : 1)
                }
                if let authorReturn {
                    PnLPill(text: authorReturn.text, tone: authorReturn.tone.pill)
                }
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}
