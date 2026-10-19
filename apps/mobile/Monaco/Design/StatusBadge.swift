import SwiftUI

enum PillMetrics {
    static let horizontal: CGFloat = 6
    static let vertical: CGFloat = 4
}

struct StatusBadge: View {
    enum Tone: CaseIterable, Equatable {
        case positive
        case negative
        case warn
        case neutral
        case attention
        case discussion

        var fill: Color {
            switch self {
            case .positive: return MonacoTheme.brandSoft
            case .negative: return MonacoTheme.sellWash
            case .warn: return MonacoTheme.warningWash
            case .neutral: return MonacoTheme.surfaceSunken
            case .attention: return MonacoTheme.voteNeededFill
            case .discussion: return MonacoTheme.discussionWash
            }
        }

        var label: Color {
            switch self {
            case .positive: return MonacoTheme.brand
            case .negative: return MonacoTheme.loss
            case .warn: return MonacoTheme.warning
            case .neutral: return MonacoTheme.secondaryText
            case .attention: return MonacoTheme.voteNeededLabel
            case .discussion: return MonacoTheme.discussionTint
            }
        }
    }

    private let text: String
    private let tone: Tone

    init(_ text: String, tone: Tone) {
        self.text = text
        self.tone = tone
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(tone.label)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.vertical, MonacoTheme.Space.xs)
            .padding(.horizontal, MonacoTheme.Space.s)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous).fill(tone.fill))
    }
}

struct PnLPill: View {
    enum Tone: Equatable {
        case gain
        case loss
        case muted

        var color: Color {
            switch self {
            case .gain: return MonacoTheme.profit
            case .loss: return MonacoTheme.loss
            case .muted: return MonacoTheme.secondaryText
            }
        }
    }

    private let text: String
    private let tone: Tone

    init(text: String, tone: Tone) {
        self.text = text
        self.tone = tone
    }

    var spokenText: String {
        guard tone != .muted else { return text }
        let parts = text.components(separatedBy: " · ")
        guard let figure = parts.first else { return text }
        let spoken = PnLSpeech.percent(figure)
        guard spoken != "unavailable" else { return text }
        return ([spoken] + parts.dropFirst()).joined(separator: ", ")
    }

    var body: some View {
        Text(text)
            .moneyFont(.caption)
            .foregroundStyle(tone.color)
            .lineLimit(1)
            .padding(.vertical, PillMetrics.vertical)
            .padding(.horizontal, PillMetrics.horizontal)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous)
                    .fill(MonacoTheme.brandSoft)
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spokenText)
    }
}

struct EventPill: View {
    enum Kind: CaseIterable, Equatable {
        case proposal
        case cabal
        case buy
        case sell
        case priceMove
        case approved
        case discussion

        var title: String {
            switch self {
            case .proposal: return "Proposal"
            case .cabal: return "Cabal"
            case .buy: return "Buy"
            case .sell: return "Sell"
            case .priceMove: return "Price move"
            case .approved: return "Approved"
            case .discussion: return "Discussion"
            }
        }

        var fill: Color {
            switch self {
            case .proposal, .cabal: return StatusBadge.Tone.positive.fill
            case .buy: return MonacoTheme.buyWash
            case .sell: return StatusBadge.Tone.negative.fill
            case .priceMove, .approved: return StatusBadge.Tone.warn.fill
            case .discussion: return StatusBadge.Tone.discussion.fill
            }
        }

        var label: Color {
            switch self {
            case .proposal, .cabal: return StatusBadge.Tone.positive.label
            case .buy: return MonacoTheme.profit
            case .sell: return StatusBadge.Tone.negative.label
            case .priceMove, .approved: return StatusBadge.Tone.warn.label
            case .discussion: return StatusBadge.Tone.discussion.label
            }
        }
    }

    private let kind: Kind

    init(kind: Kind) {
        self.kind = kind
    }

    var body: some View {
        Text(kind.title)
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(kind.label)
            .lineLimit(1)
            .padding(.vertical, PillMetrics.vertical)
            .padding(.horizontal, PillMetrics.horizontal)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous).fill(kind.fill))
    }
}
