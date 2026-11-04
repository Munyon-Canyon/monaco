import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposalHero: View {
    let summary: ProposalSummary
    let asset: ProposalAsset?
    let members: [ProposalMember]
    let paused: Bool
    var subtitle: String?
    let facts: ProposalFacts?
    var onInfo: () -> Void = {}

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var state: ProposalState { summary.state(now: .now) }
    private var proposer: ProposalMember? { members.first { $0.id == summary.proposerID } }
    private var cabalLabel: String { summary.cabalName.isEmpty ? "Your cabal" : summary.cabalName }
    private var kindLabel: String { summary.kind == "sell" ? "Sell" : "Buy" }
    private var ruleChange: ProposalRuleChange? { summary.isRule ? summary.ruleChange : nil }
    private var ticker: String {
        asset?.kind == .preIpo
            ? summary.assetName(asset: asset)
            : AssetSymbolFormatter.display(summary.symbol, kind: asset?.kind ?? .stock)
    }

    static let avatarSize: CGFloat = 32
    static let chipHeight: CGFloat = 28
    static let chipMark: CGFloat = 16
    static let infoGlyph: CGFloat = 16

    static func firstName(of name: String?) -> String? {
        name?.split(whereSeparator: \.isWhitespace).first.map(String.init)
    }

    static func proposedLabel(firstName: String, createdAt: Date, now: Date, calendar: Calendar) -> String {
        let elapsed = now.timeIntervalSince(createdAt)
        guard elapsed < 86_400 else {
            let day = ProposalStats.crestAge(since: createdAt, now: now, calendar: calendar)
            return "\(firstName) proposed this on \(day)"
        }
        let minutes = max(0, Int(elapsed / 60))
        if minutes < 1 { return "\(firstName) proposed this just now" }
        if minutes < 60 { return "\(firstName) proposed this \(minutes) \(minutes == 1 ? "minute" : "minutes") ago" }
        let hours = minutes / 60
        return "\(firstName) proposed this \(hours) \(hours == 1 ? "hour" : "hours") ago"
    }

    static func amountLabel(micros: Int64) -> String {
        micros % 1_000_000 == 0
            ? UsdAmountFormatter.format(micros: micros).replacingOccurrences(of: ".00", with: "")
            : UsdAmountFormatter.format(micros: micros)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            crest
            heroRow
            if paused && state.isOpen {
                Text(ProposalCardCopy.pausedCaption(isSell: summary.kind == "sell"))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("proposal-paused-caption")
            }
        }
    }

    @ViewBuilder private var crest: some View {
        if proposer == nil || Self.firstName(of: proposer?.name) == nil {
            cabalChip.frame(maxWidth: .infinity, alignment: .leading)
        } else if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                proposerGroup
                cabalChip
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            HStack(spacing: MonacoTheme.Space.s) {
                proposerGroup
                Spacer(minLength: MonacoTheme.Space.s)
                cabalChip.containerRelativeFrame(.horizontal, alignment: .trailing) { width, _ in width / 2 }
            }
        }
    }

    @ViewBuilder private var proposerGroup: some View {
        if let proposer, let firstName = Self.firstName(of: proposer.name) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: proposer.photoURL?.absoluteString, displayName: proposer.name, size: Self.avatarSize,
                    seed: proposer.id)
                Text(firstName)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Text(ProposalStats.crestAge(since: summary.createdAt, now: .now, calendar: .current))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            }
            .fixedSize(horizontal: true, vertical: false)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(
                Self.proposedLabel(firstName: firstName, createdAt: summary.createdAt, now: .now, calendar: .current))
        }
    }

    private var cabalChip: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: summary.cabalID))) {
            HStack(spacing: MonacoTheme.Space.s) {
                CabalMark(groupId: summary.cabalID, name: cabalLabel, size: Self.chipMark)
                Text(cabalLabel)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            .padding(.horizontal, MonacoTheme.Space.sm)
            .padding(.vertical, MonacoTheme.Space.xs)
            .frame(minHeight: Self.chipHeight)
            .background(MonacoTheme.surface, in: Capsule())
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(cabalLabel), opens the cabal")
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("proposal-crest-line")
    }

    @ViewBuilder private var heroRow: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    mark
                    VStack(alignment: .leading, spacing: 0) {
                        title
                        subline
                    }
                    amount
                }
                .accessibilityElement(children: .combine)
            } else {
                HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
                    mark
                    VStack(alignment: .leading, spacing: 0) {
                        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                            title
                            Spacer(minLength: MonacoTheme.Space.s)
                            amount
                        }
                        subline
                    }
                }
                .accessibilityElement(children: .combine)
            }
            if let facts { factsRow(facts) }
        }
    }

    @ViewBuilder private var mark: some View {
        if summary.isRule {
            SunkenGlyphMark(icon: .thesis, size: StockMarkSize.review.rawValue)
        } else {
            stockMark
        }
    }

    private var stockMark: some View {
        StockMark(
            symbol: summary.symbol, displayName: summary.assetName(asset: asset), assetKind: asset?.kind ?? .stock,
            size: .review, logoURL: asset?.logoURL)
    }

    private var title: some View {
        Text(summary.isRule ? ProposalRuleCopy.heroTitle : "\(kindLabel) \(ticker)")
            .font(MonacoTheme.Typo.title)
            .foregroundStyle(MonacoTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
    }

    @ViewBuilder private var amount: some View {
        if let ruleChange {
            Text(ruleChange.changeLine)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.trailing)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("proposal-hero-rule-change")
        } else if let micros = amountMicros {
            Text(Self.amountLabel(micros: micros))
                .moneyFigure(.card)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityIdentifier("proposal-hero-amount")
        }
    }

    @ViewBuilder private var subline: some View {
        let isAccessibility = dynamicTypeSize.isAccessibilitySize
        if let text = subtitle ?? ruleChange?.title {
            let line = Text(text)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(isAccessibility ? nil : 1)
                .minimumScaleFactor(isAccessibility ? 1 : 0.75)
                .fixedSize(horizontal: false, vertical: isAccessibility)
            if subtitle != nil {
                line.accessibilityIdentifier("proposal-hero-subtitle")
            } else {
                line
            }
        }
    }

    private var amountMicros: Int64? {
        if summary.kind == "sell" {
            return summary.quoteOutAmount > 0 ? TokenAmount.int64(summary.quoteOutAmount) : nil
        }
        return summary.usdcMicros
    }
}

extension ProposalHero {
    @ViewBuilder private func factsRow(_ facts: ProposalFacts) -> some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                stackedFacts(facts)
            } else {
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                        estimateLine(facts)
                        Spacer(minLength: MonacoTheme.Space.s)
                        treasuryLine(facts)
                    }
                    stackedFacts(facts)
                }
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(facts.spoken)
        .accessibilityAction(named: Self.infoLabel(facts.info)) { onInfo() }
        .accessibilityIdentifier("proposal-facts")
    }

    private func stackedFacts(_ facts: ProposalFacts) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            estimateLine(facts)
            treasuryLine(facts)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private func estimateLine(_ facts: ProposalFacts) -> some View {
        if let estimate = facts.estimate {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
                Text(estimate)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .monospacedDigit()
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("proposal-facts-estimate")
                if facts.info != nil {
                    Button(action: onInfo) {
                        Image(systemName: MonacoIcon.info.symbol)
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .padding(-(MonacoButtonMetrics.hitTarget - Self.infoGlyph) / 2)
                    .accessibilityLabel(Self.infoLabel(facts.info))
                    .accessibilityIdentifier("proposal-facts-info")
                }
            }
        }
    }

    @ViewBuilder private func treasuryLine(_ facts: ProposalFacts) -> some View {
        if let treasury = facts.treasury {
            Text(treasury)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .monospacedDigit()
                .lineLimit(1)
                .fixedSize()
                .accessibilityIdentifier("proposal-facts-treasury")
        }
    }

    static func infoLabel(_ info: ProposalFacts.Info?) -> String {
        info == .estimate ? "About the estimate" : "About price"
    }
}
