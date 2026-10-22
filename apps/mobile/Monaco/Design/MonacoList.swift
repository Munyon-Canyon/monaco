import SwiftUI

enum SectionHeaderMetrics {
    static let countSize: CGFloat = 24
    static let chevronSize: CGFloat = 13
    static let hitHeight: CGFloat = 44
}

/// Sentence-case section title with an optional trailing text button ("See all").
struct MonacoSectionHeader: View {
    private let title: String
    private let trailing: String?
    private let action: (() -> Void)?

    init(_ title: String, trailing: String? = nil, action: (() -> Void)? = nil) {
        self.title = title
        self.trailing = trailing
        self.action = action
    }

    /// A count beside the title — "Needs your vote" with a `2` — for a section that is an errand.
    private var count: Int?
    private var actionIdentifier: String?

    init(
        _ title: String, count: Int? = nil, trailing: String? = nil, actionIdentifier: String? = nil,
        action: (() -> Void)? = nil
    ) {
        self.title = title
        self.count = count
        self.trailing = trailing
        self.actionIdentifier = actionIdentifier
        self.action = action
    }

    @ScaledMetric(relativeTo: .subheadline) private var countSize = SectionHeaderMetrics.countSize
    @ScaledMetric(relativeTo: .subheadline) private var chevronSize = SectionHeaderMetrics.chevronSize

    var body: some View {
        HStack(spacing: 0) {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(title)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityAddTraits(.isHeader)
                if let count, count > 0 {
                    Text("\(count)")
                        .font(MonacoTheme.Typo.captionStrong.monospacedDigit())
                        .foregroundStyle(MonacoTheme.brand)
                        .padding(.horizontal, count > 9 ? MonacoTheme.Space.xs : 0)
                        .frame(minWidth: countSize, minHeight: countSize)
                        .background(Capsule().fill(MonacoTheme.brandSoft))
                        .accessibilityLabel("\(count)")
                }
            }
            Spacer(minLength: MonacoTheme.Space.s)
            if let trailing {
                if let action {
                    Button(action: action) {
                        HStack(spacing: MonacoTheme.Space.xs) {
                            Text(trailing)
                                .font(MonacoTheme.Typo.captionStrong)
                                .lineLimit(1)
                            Image(systemName: "chevron.right")
                                .font(.system(size: chevronSize, weight: .semibold))
                                .accessibilityHidden(true)
                        }
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minHeight: SectionHeaderMetrics.hitHeight)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier(actionIdentifier ?? "")
                } else {
                    Text(trailing)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .lineLimit(1)
                }
            }
        }
        .frame(minHeight: SectionHeaderMetrics.hitHeight)
        .padding(.bottom, MonacoTheme.Space.sm)
    }
}

/// A run of `MonacoRow`s: their own hairlines between them, nothing above the first or below the
/// last, no surface and no radius.
struct MonacoGroupedList<Content: View>: View {
    private let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        VStack(spacing: 0) {
            content
        }
        .frame(maxWidth: .infinity)
    }
}

/// A one-pixel hairline, full width: 0.5pt on 2x, 0.33pt on 3x.
struct MonacoRule: View {
    var color: Color = MonacoTheme.border
    @Environment(\.displayScale) private var displayScale

    var body: some View {
        Rectangle()
            .fill(color)
            .frame(height: 1 / displayScale)
            .accessibilityHidden(true)
    }
}

/// Leading 44pt mark, title over subtitle, trailing figures. Wrap in a `Button` or `NavigationLink`
/// with `.buttonStyle(.monacoRow)` for the pressed state.
struct MonacoRow<Leading: View, Trailing: View>: View {
    private let title: String
    private let titleFont: Font
    private let titleColor: Color
    private let subtitle: String?
    private let subtitleColor: Color
    private let chevron: Bool
    private let isLast: Bool
    private let trailingIsInteractive: Bool
    private let isPrivate: Bool
    private let leading: Leading
    private let trailing: Trailing

    /// `titleFont` is the brand's row title unless the row is a stock, whose label is its ticker
    /// and sets in `MonacoTheme.Typo.ticker`.
    init(
        title: String,
        titleFont: Font = MonacoTheme.Typo.rowTitle,
        titleColor: Color = MonacoTheme.primaryText,
        subtitle: String? = nil,
        subtitleColor: Color = MonacoTheme.secondaryText,
        chevron: Bool = false,
        isLast: Bool = false,
        trailingIsInteractive: Bool = false,
        isPrivate: Bool = false,
        @ViewBuilder leading: () -> Leading,
        @ViewBuilder trailing: () -> Trailing
    ) {
        self.title = title
        self.titleFont = titleFont
        self.titleColor = titleColor
        self.subtitle = subtitle
        self.subtitleColor = subtitleColor
        self.chevron = chevron
        self.isLast = isLast
        self.trailingIsInteractive = trailingIsInteractive
        self.isPrivate = isPrivate
        self.leading = leading()
        self.trailing = trailing()
    }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body)
    private var titleWidthFloor: CGFloat = MonacoRowLayout.baseMinimumTitleWidth

    /// True for the `Trailing == EmptyView` overload, where there are no figures to lay out.
    private var hasTrailing: Bool { Trailing.self != EmptyView.self }

    private var layout: MonacoRowLayout {
        MonacoRowLayout(dynamicTypeSize: dynamicTypeSize, scaledTitleWidthFloor: titleWidthFloor)
    }

    private var labels: some View {
        MonacoRowLabels(
            title: title, titleFont: titleFont, titleColor: titleColor, subtitle: subtitle,
            subtitleColor: subtitleColor, layout: layout
        ) {}
        .monacoPrivate(isPrivate)
    }

    /// Mark, then title over subtitle, then the figures. At accessibility text sizes the figures
    /// drop below the labels instead of squeezing them out of the row.
    private var content: some View {
        Group {
            if layout.isStacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        leading
                            .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                        labels
                        if chevron { MonacoRowChevron() }
                    }
                    // Chevron-only rows have no second line to drop below the labels; an empty
                    // column would still spend the stack's spacing.
                    if hasTrailing {
                        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                            trailing
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            } else {
                HStack(spacing: MonacoTheme.Space.sm) {
                    leading
                        .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                    // The labels keep a floor and truncate; the figures take what is left and
                    // shrink through MoneyText's minimumScaleFactor before they ever truncate.
                    labels
                        .frame(minWidth: layout.minimumTitleWidth, alignment: .leading)
                    VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                        trailing
                    }
                    .layoutPriority(1)
                    if chevron { MonacoRowChevron() }
                }
            }
        }
    }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule()
                        .padding(.leading, layout.separatorLeadingInset)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                }
            }
            .accessibilityElement(children: trailingIsInteractive ? .contain : .combine)
    }
}

struct MonacoRowLabels<Extra: View>: View {
    let title: String
    var titleFont: Font = MonacoTheme.Typo.rowTitle
    var titleColor: Color = MonacoTheme.primaryText
    var subtitle: String?
    var subtitleColor: Color = MonacoTheme.secondaryText
    let layout: MonacoRowLayout
    @ViewBuilder var extra: Extra

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(title)
                .font(titleFont)
                .foregroundStyle(titleColor)
                .lineLimit(layout.titleLineLimit)
                .truncationMode(.tail)
            extra
            if let subtitle, !subtitle.isEmpty {
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(subtitleColor)
                    .lineLimit(layout.subtitleLineLimit)
                    .truncationMode(.tail)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct MonacoRowChevron: View {
    @ScaledMetric(relativeTo: .body) private var size: CGFloat = MonacoRowLayout.chevronSize

    var body: some View {
        MonacoGlyph(.chevron, size: size)
            .foregroundStyle(MonacoTheme.secondaryText)
    }
}

/// How a `MonacoRow` arranges itself for the current text size.
///
/// The trailing column used to be `fixedSize`, so it was always offered its ideal width. At
/// accessibility sizes a figure like "$12,480.55" is wider than the whole row, which left the
/// title zero width (an ellipsis) and stopped `MoneyText`'s `minimumScaleFactor` from ever
/// applying. Rows now stack at accessibility sizes, and the figures shrink at normal sizes.
struct MonacoRowLayout: Equatable {
    /// Floor for the label column at the default text size.
    static let baseMinimumTitleWidth: CGFloat = 96

    let isStacked: Bool
    private let scaledTitleWidthFloor: CGFloat

    init(
        dynamicTypeSize: DynamicTypeSize,
        scaledTitleWidthFloor: CGFloat = MonacoRowLayout.baseMinimumTitleWidth
    ) {
        isStacked = dynamicTypeSize.isAccessibilitySize
        self.scaledTitleWidthFloor = scaledTitleWidthFloor
    }

    /// A stacked row gives the title room for two lines; an inline row still truncates at one.
    var titleLineLimit: Int { isStacked ? 2 : 1 }

    var subtitleLineLimit: Int { isStacked ? 2 : 1 }

    /// Floor for the label column in the inline layout, so the figures give way first. It scales
    /// with the title: the title grows up to xxxLarge while the row is still inline, and a fixed
    /// 96pt is about five characters at that size, so a row with a wide figure went on truncating.
    var minimumTitleWidth: CGFloat? { isStacked ? nil : scaledTitleWidthFloor }

    static let baseMarkSize: CGFloat = 44

    static let minHeight: CGFloat = 72

    static let chevronSize: CGFloat = 16

    /// The rule spans the row's content width, from the left gutter to the right one.
    var separatorLeadingInset: CGFloat { 0 }

    /// The inset for a row that still starts its rule under the labels, such as the stock and
    /// board rows: derived from the mark size rather than written down.
    func separatorLeadingInset(markSize: CGFloat) -> CGFloat {
        guard !isStacked else { return MonacoTheme.Space.gutter }
        return MonacoTheme.Space.gutter + markSize + MonacoTheme.Space.sm
    }
}

extension MonacoRow where Trailing == EmptyView {
    init(
        title: String,
        titleFont: Font = MonacoTheme.Typo.rowTitle,
        titleColor: Color = MonacoTheme.primaryText,
        subtitle: String? = nil,
        subtitleColor: Color = MonacoTheme.secondaryText,
        chevron: Bool = false,
        isLast: Bool = false,
        isPrivate: Bool = false,
        @ViewBuilder leading: () -> Leading
    ) {
        self.init(
            title: title,
            titleFont: titleFont,
            titleColor: titleColor,
            subtitle: subtitle,

            subtitleColor: subtitleColor,
            chevron: chevron,
            isLast: isLast,
            isPrivate: isPrivate,
            leading: leading,
            trailing: { EmptyView() }
        )
    }
}

/// Pressed state for a tappable `MonacoRow`: sunken fill, no scale.
struct MonacoRowButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(configuration.isPressed ? MonacoTheme.surfaceSunken : Color.clear)
    }
}

extension ButtonStyle where Self == MonacoRowButtonStyle {
    static var monacoRow: MonacoRowButtonStyle { MonacoRowButtonStyle() }
}

struct MonacoRowSkeleton: View {
    enum MarkShape {
        case tile
        case circle
        case none
    }

    var rows: Int = 3
    var markShape: MarkShape = .tile
    var hasTrailing: Bool = true

    var body: some View {
        VStack(spacing: 0) {
            ForEach(0..<rows, id: \.self) { index in
                HStack(spacing: MonacoTheme.Space.sm) {
                    mark
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonFraction(SkeletonMetrics.rowTitleFraction) { line }
                        SkeletonFraction(SkeletonMetrics.rowSubtitleFraction) { line }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    if hasTrailing {
                        SkeletonBlock(
                            width: SkeletonMetrics.rowTrailingWidth, height: SkeletonMetrics.lineHeight,
                            radius: SkeletonMetrics.lineRadius)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.sm)
                .frame(minHeight: MonacoRowLayout.minHeight)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter)
                    }
                }
            }
        }
        .accessibilityHidden(true)
    }

    private var line: some View {
        SkeletonBlock(height: SkeletonMetrics.lineHeight, radius: SkeletonMetrics.lineRadius)
    }

    @ViewBuilder private var mark: some View {
        let size = MonacoRowLayout.baseMarkSize
        switch markShape {
        case .tile: SkeletonBlock(width: size, height: size, radius: MonacoTheme.Radius.tile)
        case .circle: SkeletonBlock(width: size, height: size, radius: size / 2)
        case .none: EmptyView()
        }
    }
}

enum EmptyStateMetrics {
    static let tile: CGFloat = 68
    static let tileRadius: CGFloat = 24
    static let glyph: CGFloat = 30
    static let messageWidth: CGFloat = 280
    static let readableWidth: CGFloat = 480
}

/// Empty state: an optional 68pt icon tile, a 22pt title, a muted message and up to two full-width
/// buttons. Centred; neither line truncates.
///
/// No accessibility container on purpose: call sites put their identifier on this view and UI
/// tests find the retry button by it, which only works while the identifier reaches the button.
struct EmptyState: View {
    private let title: String
    private let message: String?
    private let actionTitle: String?
    private let actionIdentifier: String?
    private let action: (() -> Void)?
    private let systemImage: String?
    private let secondaryActionTitle: String?
    private let secondaryAction: (() -> Void)?

    init(
        title: String, message: String? = nil, actionTitle: String? = nil, actionIdentifier: String? = nil,
        isOnlyContent: Bool = false, systemImage: String? = nil, secondaryActionTitle: String? = nil,
        action: (() -> Void)? = nil, secondaryAction: (() -> Void)? = nil
    ) {
        self.title = title
        self.message = message
        self.actionTitle = actionTitle
        self.actionIdentifier = actionIdentifier
        self.action = action
        self.systemImage = systemImage
        self.secondaryActionTitle = secondaryActionTitle
        self.secondaryAction = secondaryAction
    }

    static func loadFailed(identifier: String, retry: @escaping () -> Void) -> EmptyState {
        EmptyState(
            title: "Couldn't load this view",
            message: "Check your connection and try again.",
            actionTitle: "Try again",
            actionIdentifier: identifier,
            systemImage: "wifi.exclamationmark",
            action: retry
        )
    }

    @ScaledMetric(relativeTo: .title2) private var glyphSize = EmptyStateMetrics.glyph

    @ViewBuilder private var glyph: some View {
        if let icon = systemImage.flatMap(MonacoIcon.init(systemName:)), icon.hasPixel {
            MonacoGlyph(icon, size: glyphSize)
        } else {
            Image(systemName: systemImage ?? "")
                .font(.system(size: glyphSize, weight: .semibold))
        }
    }

    private var tile: some View {
        glyph
            .foregroundStyle(MonacoTheme.brand)
            .frame(width: EmptyStateMetrics.tile, height: EmptyStateMetrics.tile)
            .background(
                RoundedRectangle(cornerRadius: EmptyStateMetrics.tileRadius, style: .continuous)
                    .fill(MonacoTheme.brandSoft)
            )
            .accessibilityHidden(true)
    }

    @ViewBuilder private var primaryButton: some View {
        if let actionTitle, let action {
            let button = Button(actionTitle, action: action)
                .buttonStyle(.monacoPrimary)
            if let actionIdentifier {
                button.accessibilityIdentifier(actionIdentifier)
            } else {
                button
            }
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            if systemImage != nil {
                tile.padding(.bottom, MonacoTheme.Space.m)
            }
            Text(title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)

            if let message, !message.isEmpty {
                Text(message)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: EmptyStateMetrics.messageWidth)
                    .padding(.top, MonacoTheme.Space.s)
            }
            if actionTitle != nil, action != nil {
                VStack(spacing: MonacoTheme.Space.sm) {
                    primaryButton
                    if let secondaryActionTitle, let secondaryAction {
                        Button(secondaryActionTitle, action: secondaryAction)
                            .buttonStyle(.monacoOutline)
                    }
                }
                .padding(.top, MonacoTheme.Space.l)
            }
        }
        .frame(maxWidth: EmptyStateMetrics.readableWidth)
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.top, MonacoTheme.Space.xl)
        .padding(.bottom, MonacoTheme.Space.xl)
    }
}

/// "Couldn't load holdings." with a trailing "Try again", for a slot that has nothing on screen.
/// The default style is an error notice; `.card` is the partial-error card for a section whose
/// siblings still show.
struct MonacoErrorRow: View {
    enum Style {
        case notice
        case card
    }

    let thing: String
    let identifier: String
    var inset = true
    var style: Style = .notice
    let retry: () -> Void

    private var retryIdentifier: String {
        let suffix = ["-error", "-failed", "-retry"].first(where: identifier.hasSuffix)
        let stem = suffix.map { String(identifier.dropLast($0.count)) } ?? identifier
        return "\(stem)-retry"
    }

    private var containerIdentifier: String {
        identifier == retryIdentifier ? "\(identifier.dropLast(6))-error" : identifier
    }

    @ScaledMetric(relativeTo: .title3) private var cardGlyph = MonacoErrorMetrics.cardGlyph

    var body: some View {
        Group {
            switch style {
            case .notice: notice
            case .card: card
            }
        }
        .padding(.horizontal, inset ? MonacoTheme.Space.gutter : 0)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(containerIdentifier)
    }

    private var notice: some View {
        MonacoNotice(
            .error, message: "Couldn't load \(thing).", action: ("Try again", retry),
            actionIdentifier: retryIdentifier
        )
        .padding(.vertical, MonacoTheme.Space.s)
    }

    private var card: some View {
        MonacoCard {
            VStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: "wifi.exclamationmark")
                    .font(.system(size: cardGlyph, weight: .semibold))
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                Text("Couldn't load \(thing)")
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .multilineTextAlignment(.center)
                Text("Your other sections are still up to date.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                Button("Try again", action: retry)
                    .buttonStyle(.monacoCompact)
                    .accessibilityIdentifier(retryIdentifier)
            }
            .frame(maxWidth: .infinity)
        }
        .padding(.vertical, MonacoTheme.Space.s)
    }
}

enum MonacoErrorMetrics {
    static let cardGlyph: CGFloat = 22
}

struct SolscanLinkRow: View {
    let url: URL
    var name = "Solscan"
    let identifier: String

    var body: some View {
        Link(destination: url) {
            HStack(spacing: MonacoTheme.Space.sm) {
                Text("View on \(name)")
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Spacer(minLength: MonacoTheme.Space.sm)
                Image(systemName: "arrow.up.right")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier(identifier)
    }
}
