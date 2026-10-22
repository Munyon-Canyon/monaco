import SwiftUI

enum OutcomeMetrics {
    static let orb: CGFloat = 72
    static let orbRadius: CGFloat = 24
    static let glyph: CGFloat = 32
    static let messageWidth: CGFloat = 290
    static let titleGap: CGFloat = 20
}

struct OutcomeView<Content: View>: View {
    enum Tone: Equatable {
        case done
        case pending
        case error

        var fill: Color {
            switch self {
            case .done: return MonacoTheme.brandSoft
            case .pending: return MonacoTheme.warningWash
            case .error: return MonacoTheme.sellWash
            }
        }

        var accent: Color {
            switch self {
            case .done: return MonacoTheme.brand
            case .pending: return MonacoTheme.warning
            case .error: return MonacoTheme.loss
            }
        }

        var icon: MonacoIcon {
            switch self {
            case .done: .check
            case .pending: .clock
            case .error: .alert
            }
        }

        var symbol: String { icon.symbol }
    }

    private let tone: Tone
    private let title: String
    private let message: String
    private let content: Content

    init(tone: Tone, title: String, message: String, @ViewBuilder content: () -> Content) {
        self.tone = tone
        self.title = title
        self.message = message
        self.content = content()
    }

    @ScaledMetric(relativeTo: .title) private var glyphSize = OutcomeMetrics.glyph

    var body: some View {
        VStack(spacing: 0) {
            MonacoGlyph(tone.icon, size: glyphSize, weight: .bold)
                .foregroundStyle(tone.accent)
                .monacoStamp()
                .frame(width: OutcomeMetrics.orb, height: OutcomeMetrics.orb)
                .background(
                    RoundedRectangle(cornerRadius: OutcomeMetrics.orbRadius, style: .continuous).fill(tone.fill)
                )
                .accessibilityHidden(true)
            Text(title)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, OutcomeMetrics.titleGap)
                .accessibilityAddTraits(.isHeader)
            Text(message)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: OutcomeMetrics.messageWidth)
                .padding(.top, MonacoTheme.Space.s)
                .monacoPrivate()
            content
                .frame(maxWidth: .infinity)
                .padding(.top, MonacoTheme.Space.l)
        }
        .frame(maxWidth: .infinity)
        .onAppear {
            AccessibilityNotification.Announcement(title).post()
        }
    }
}

extension OutcomeView where Content == EmptyView {
    init(tone: Tone, title: String, message: String) {
        self.init(tone: tone, title: title, message: message) { EmptyView() }
    }
}
