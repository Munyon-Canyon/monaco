import MonacoCore
import SwiftUI

struct ProposalCard: View {
    private let summary: ProposalSummary
    let asset: ProposalAsset?
    let members: [ProposalMember]
    private let paused: Bool
    private let openRoute: AnyAppRoute?
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(
        proposal: ProposalSummary, asset: ProposalAsset?, members: [ProposalMember], paused: Bool = false,
        showsThesis: Bool = true,
        isDetail: Bool = false,
        showsTracker: Bool = true,
        openRoute: AnyAppRoute? = nil,
        vote: @escaping (String) -> Void = { _ in }
    ) {
        summary = proposal
        self.asset = asset
        self.members = members
        self.paused = paused
        self.openRoute = openRoute
    }

    init(
        proposal: ProposalDetail, asset: ProposalAsset?, members: [ProposalMember], paused: Bool = false,
        showsThesis: Bool = true,
        isDetail: Bool = false,
        showsTracker: Bool = true,
        openRoute: AnyAppRoute? = nil,
        vote: @escaping (String) -> Void = { _ in }
    ) {
        self.init(
            proposal: proposal.summary, asset: asset, members: members, paused: paused, openRoute: openRoute)
    }

    private var state: ProposalState { summary.state(now: .now) }
    private var isSell: Bool { summary.kind == "sell" }
    private var proposerName: String { members.first { $0.id == summary.proposerID }?.name ?? "Member" }
    private var headline: String { summary.cabalName.isEmpty ? proposerName : summary.cabalName }
    private var action: ProposalCardCopy.Action { ProposalCardCopy.button(state) }
    private var note: String {
        ProposalCardCopy.note(
            state, needed: summary.tally.needed, expiresAt: summary.expiresAt, statusMessage: summary.statusMessage,
            now: .now)
    }
    private var footer: ProposalCardCopy.Footer { ProposalCardCopy.footer(state, tally: summary.tally) }

    private var amountLine: String {
        if isSell { return "\(Self.shareLabel(summary: summary, asset: asset)) from the cabal's holding" }
        guard let micros = summary.usdcMicros else { return "From the cabal pot" }
        return "\(UsdAmountFormatter.format(micros: micros)) from the cabal pot"
    }

    private var spokenLabel: String {
        "\(headline): \(ProposalBadge.card(state).label). \(summary.title(asset: asset)), \(amountLine). \(note)"
    }

    var body: some View {
        MonacoCard(style: .inset) {
            VStack(alignment: .leading, spacing: 0) {
                topRow
                    .padding(.top, MonacoTheme.Space.sm)
                instrumentRow
                    .padding(.top, MonacoTheme.Space.sm)
                Text(note)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, MonacoTheme.Space.sm)
                    .padding(.bottom, MonacoTheme.Space.s)
                    .accessibilityIdentifier(state.isOpen ? "proposal-closes-in" : "proposal-card-note")
                if paused && state.isOpen {
                    Text(ProposalCardCopy.pausedCaption(isSell: isSell))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .padding(.bottom, MonacoTheme.Space.s)
                        .accessibilityIdentifier("proposal-paused-caption")
                }
                footerLink
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(spokenLabel)
        .accessibilityAction(named: action.title) { open() }
        .accessibilityAction(named: "Open proposal") { open() }
        .accessibilityIdentifier("proposal-card-\(summary.id)")
    }

    private var topRow: some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let layout =
            stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return layout {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: members.first { $0.id == summary.proposerID }?.photoURL?.absoluteString,
                    displayName: proposerName, size: 24, seed: summary.proposerID)
                Text(headline)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .lineLimit(stacked ? 2 : 1)
                    .truncationMode(.tail)
                    .monacoPrivate()
            }
            if !stacked {
                Spacer(minLength: MonacoTheme.Space.s)
            }
            StatusBadge(proposal: .card(state))
                .layoutPriority(1)
                .accessibilityIdentifier("proposal-status-chip")
        }
    }

    private var instrumentRow: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(
                    symbol: summary.symbol, displayName: asset?.displayName, assetKind: asset?.kind ?? .stock,
                    size: .card, logoURL: asset?.logoURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(summary.title(asset: asset))
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(amountLine)
                        .monacoPrivate()
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            if let openRoute {
                reviewButton(openRoute)
            }
        }
    }

    @ViewBuilder private func reviewButton(_ route: AnyAppRoute) -> some View {
        let link = NavigationLink(value: route) { Text(action.title) }
        if action.isPrimary {
            link.buttonStyle(.monacoCompactProminent)
        } else {
            link.buttonStyle(.monacoCompact)
        }
    }

    private var footerLink: some View {
        VStack(spacing: 0) {
            MonacoRule()
            if let openRoute {
                NavigationLink(value: openRoute) { footerRow }
                    .buttonStyle(.plain)
            } else {
                footerRow
            }
        }
    }

    private var footerRow: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(monaco: footer.icon.monacoIcon)
                .accessibilityHidden(true)
            Text(footer.text)
                .frame(maxWidth: .infinity, alignment: .leading)
                .fixedSize(horizontal: false, vertical: true)
            MonacoRowChevron()
        }
        .font(MonacoTheme.Typo.caption)
        .foregroundStyle(footer.tone.color)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
    }

    private func open() {
        guard openRoute != nil, let navigator = environment?.navigator else { return }
        navigator.open(ProposalRoute(proposalID: summary.id), in: navigator.selectedTab)
    }

    static func shareLabel(summary: ProposalSummary, asset: ProposalAsset?) -> String {
        guard let amount = summary.tokenAmount else { return "0 shares" }
        return TokenQuantityFormatter.label(
            fromAtomics: String(amount), decimals: asset?.decimals ?? AssetCatalogDefaults.decimals,
            kind: asset?.kind ?? .stock)
    }
}

extension ProposalCardCopy.FooterIcon {
    fileprivate var monacoIcon: MonacoIcon {
        switch self {
        case .chat: .chat
        case .clock: .clock
        case .done: .checkCircle
        case .warning: .alert
        }
    }
}

extension ProposalCardCopy.FooterTone {
    fileprivate var color: Color {
        switch self {
        case .muted: MonacoTheme.muted
        case .warning: MonacoTheme.warning
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        }
    }
}
