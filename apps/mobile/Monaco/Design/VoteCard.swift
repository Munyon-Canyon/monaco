import MonacoCore
import SwiftUI

struct VoteCard<Actions: View>: View {
    enum Mark: Equatable {
        case stock(symbol: String, name: String?, logoURL: URL?)
        case rule
    }

    let title: String
    let mark: Mark
    let line: VoteCardLine
    let yes: Int
    let no: Int
    let eligible: Int
    let identifierPrefix: String
    let identifierSuffix: String?
    var showsChevron = true
    var ruleIcon = MonacoIcon.thesis
    let onOpen: () -> Void
    @ViewBuilder let actions: () -> Actions

    static func lineWidth(screenWidth: CGFloat) -> CGFloat {
        screenWidth - 2 * MonacoTheme.Space.m - 2 * MonacoCard<EmptyView>.Style.standard.horizontalPadding - markSize
            - 2 * MonacoTheme.Space.sm - chevronWidth
    }

    static var markSize: CGFloat { 40 }
    static var chevronWidth: CGFloat { 12 }

    var body: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                header
                VoteTallyBar(yes: yes, no: no, eligible: eligible)
                actions()
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-card", identifierSuffix))
    }

    private var header: some View {
        Button(action: onOpen) {
            HStack(spacing: MonacoTheme.Space.sm) {
                markView
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .accessibilityIdentifier(
                            VoteIdentifier.make(identifierPrefix, "vote-headline", identifierSuffix))
                    lineView
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if showsChevron {
                    Image(monaco: .chevron)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityHidden(true)
                }
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityHint("Opens the proposal")
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-open", identifierSuffix))
    }

    @ViewBuilder private var markView: some View {
        switch mark {
        case .stock(let symbol, let name, let logoURL):
            StockMark(symbol: symbol, displayName: name, size: Self.markSize, logoURL: logoURL)
        case .rule:
            SunkenGlyphMark(icon: ruleIcon, size: Self.markSize)
        }
    }

    private var lineView: some View {
        ViewThatFits(in: .horizontal) {
            Text(line.full).lineLimit(1)
            Text(line.short).lineLimit(1)
            Text(line.short).fixedSize(horizontal: false, vertical: true)
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(MonacoTheme.secondaryText)
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-subhead", identifierSuffix))
    }
}

#Preview("Vote card") {
    VStack(spacing: MonacoTheme.Space.m) {
        VoteCard(
            title: "Buy NVDA \u{00B7} $400", mark: .stock(symbol: "NVDA", name: "NVIDIA", logoURL: nil),
            line: VoteCardLine(
                full: "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 6 replies \u{00B7} closes in 4h",
                short: "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 6 replies \u{00B7} 4h"),
            yes: 4, no: 1, eligible: 8, identifierPrefix: "preview", identifierSuffix: "1", onOpen: {}
        ) {}
        VoteCard(
            title: "Rule change \u{00B7} Show who voted what", mark: .rule,
            line: VoteCardLine(
                full: "5 of 8 voted \u{00B7} closes in 4h", short: "5 of 8 voted \u{00B7} 4h"),
            yes: 4, no: 1, eligible: 8, identifierPrefix: "preview", identifierSuffix: "2", onOpen: {}
        ) {}
    }
    .padding(MonacoTheme.Space.m)
}
