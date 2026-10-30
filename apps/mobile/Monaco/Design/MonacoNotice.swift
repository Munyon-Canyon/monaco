import SwiftUI

enum MonacoNoticeMetrics {
    static let icon: CGFloat = 18
    static let quietIcon: CGFloat = 16
}

struct MonacoNotice: View {
    enum Tone: Equatable {
        case info
        case warn
        case error

        var fill: Color {
            switch self {
            case .info: return MonacoTheme.brandSoft
            case .warn: return MonacoTheme.warningWash
            case .error: return MonacoTheme.sellWash
            }
        }

        var accent: Color {
            switch self {
            case .info: return MonacoTheme.brand
            case .warn: return MonacoTheme.warning
            case .error: return MonacoTheme.loss
            }
        }

        var symbol: String {
            switch self {
            case .info: return "info.circle"
            case .warn: return "clock"
            case .error: return "exclamationmark.triangle"
            }
        }

        var speechPrefix: String? {
            switch self {
            case .info: return nil
            case .warn: return "Warning:"
            case .error: return "Error:"
            }
        }
    }

    private let tone: Tone
    private let title: String?
    private let message: String
    private let action: (String, () -> Void)?
    private let actionIdentifier: String?

    init(
        _ tone: Tone, title: String? = nil, message: String, action: (String, () -> Void)? = nil,
        actionIdentifier: String? = nil
    ) {
        self.tone = tone
        self.title = title
        self.message = message
        self.action = action
        self.actionIdentifier = actionIdentifier
    }

    @ScaledMetric(relativeTo: .subheadline) private var iconSize = MonacoNoticeMetrics.icon

    var spokenText: String {
        [tone.speechPrefix, title, message].compactMap { $0 }.joined(separator: " ")
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
            Image(systemName: tone.symbol)
                .font(.system(size: iconSize, weight: .semibold))
                .foregroundStyle(tone.accent)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                if let title {
                    Text(title)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(tone.accent)
                }
                Text(message)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.primaryText)
            }
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spokenText)
            if let action {
                Button(action.0, action: action.1)
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier(actionIdentifier ?? "")
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .padding(.horizontal, MonacoTheme.Space.m)
        .background(RoundedRectangle(cornerRadius: MonacoTheme.Radius.notice, style: .continuous).fill(tone.fill))
        .accessibilityElement(children: .contain)
    }
}

struct MonacoQuietEmpty: View {
    private let text: String
    private let systemImage: String

    init(_ text: String, systemImage: String) {
        self.text = text
        self.systemImage = systemImage
    }

    @ScaledMetric(relativeTo: .subheadline) private var iconSize = MonacoNoticeMetrics.quietIcon

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(systemName: systemImage)
                .font(.system(size: iconSize, weight: .medium))
                .accessibilityHidden(true)
            Text(text)
                .font(MonacoTheme.Typo.caption)
        }
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}
