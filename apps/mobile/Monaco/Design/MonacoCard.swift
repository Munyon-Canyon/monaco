import SwiftUI

struct MonacoCard<Content: View>: View {
    enum Style: Equatable {
        case standard
        case tight
        case inset

        var radius: CGFloat {
            switch self {
            case .standard, .inset: return MonacoTheme.Radius.card
            case .tight: return MonacoTheme.Radius.cardTight
            }
        }

        var verticalPadding: CGFloat {
            switch self {
            case .standard: return MonacoTheme.Space.m
            case .tight: return MonacoTheme.Space.sm
            case .inset: return 0
            }
        }

        var horizontalPadding: CGFloat { MonacoTheme.Space.m }
    }

    private let style: Style
    private let content: Content

    init(style: Style = .standard, @ViewBuilder content: () -> Content) {
        self.style = style
        self.content = content()
    }

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: style.radius, style: .continuous)
        content
            .environment(\.monacoKeyRowsInset, style == .inset)
            .padding(.vertical, style.verticalPadding)
            .padding(.horizontal, style.horizontalPadding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(shape.fill(MonacoTheme.surface))
            .overlay(shape.strokeBorder(MonacoTheme.border, lineWidth: 1 / displayScale))
    }
}

struct MonacoKeyRows<Content: View>: View {
    private let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        VStack(spacing: 0) {
            content
        }
        .environment(\.monacoKeyRowsInset, true)
    }
}

enum MonacoDisclosureMetrics {
    static let summaryHeight: CGFloat = 48
    static let chevronRotation: Double = 180
    static let duration: Double = 0.2
}

struct MonacoDisclosureStyle: DisclosureGroupStyle {
    private static let openRotation = MonacoDisclosureMetrics.chevronRotation

    @Environment(\.displayScale) private var displayScale

    func makeBody(configuration: Configuration) -> some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
        VStack(spacing: 0) {
            Button {
                withAnimation(.easeOut(duration: MonacoDisclosureMetrics.duration)) {
                    configuration.isExpanded.toggle()
                }
            } label: {
                HStack(spacing: MonacoTheme.Space.sm) {
                    configuration.label
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .multilineTextAlignment(.leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Image(systemName: "chevron.down")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .rotationEffect(.degrees(configuration.isExpanded ? Self.openRotation : 0))
                        .accessibilityHidden(true)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .frame(minHeight: MonacoDisclosureMetrics.summaryHeight)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityValue(configuration.isExpanded ? "expanded" : "collapsed")
            if configuration.isExpanded {
                configuration.content
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .environment(\.monacoKeyRowsInset, true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(shape.strokeBorder(MonacoTheme.border, lineWidth: 1 / displayScale))
    }
}

extension DisclosureGroupStyle where Self == MonacoDisclosureStyle {
    static var monaco: MonacoDisclosureStyle { MonacoDisclosureStyle() }
}
