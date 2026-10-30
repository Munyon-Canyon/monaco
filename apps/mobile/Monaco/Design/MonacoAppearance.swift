import SwiftUI
import UIKit

/// Root appearance wiring and reusable styles for sibling screen agents.
enum MonacoAppearance {
    static func configureUIKit() {
        let canvas = UIColor(MonacoTheme.canvas)
        let primaryText = UIColor(MonacoTheme.primaryText)
        let secondaryText = UIColor(MonacoTheme.secondaryText)
        let tint = UIColor(MonacoTheme.tint)
        let border = UIColor(MonacoTheme.border)
        let titleFont = UIFont.systemFont(ofSize: 17, weight: .semibold)
        let largeTitleFont = UIFont.systemFont(ofSize: 28, weight: .bold)
        let titleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .headline).scaledFont(for: titleFont, maximumPointSize: 22),
        ]
        let largeTitleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .largeTitle).scaledFont(for: largeTitleFont, maximumPointSize: 34),
        ]

        // Chevron-only back button: the title is drawn clear and at a near-zero size so it takes no width.
        let backButton = UIBarButtonItemAppearance(style: .plain)
        backButton.normal.titleTextAttributes = [
            .foregroundColor: UIColor.clear, .font: UIFont.systemFont(ofSize: 0.1),
        ]
        backButton.highlighted.titleTextAttributes = [
            .foregroundColor: UIColor.clear, .font: UIFont.systemFont(ofSize: 0.1),
        ]
        let backImage = UIImage(
            systemName: "chevron.left", withConfiguration: UIImage.SymbolConfiguration(weight: .semibold))

        let standard = UINavigationBarAppearance()
        if #available(iOS 26, *) {
            standard.configureWithDefaultBackground()
        } else {
            standard.configureWithOpaqueBackground()
            standard.backgroundColor = canvas
            standard.shadowColor = border
        }
        standard.titleTextAttributes = titleAttributes
        standard.largeTitleTextAttributes = largeTitleAttributes
        standard.backButtonAppearance = backButton
        standard.setBackIndicatorImage(backImage, transitionMaskImage: backImage)

        // At rest (scroll edge): transparent, no hairline.
        let scrollEdge = UINavigationBarAppearance()
        scrollEdge.configureWithTransparentBackground()
        scrollEdge.titleTextAttributes = titleAttributes
        scrollEdge.largeTitleTextAttributes = largeTitleAttributes
        scrollEdge.backButtonAppearance = backButton
        scrollEdge.setBackIndicatorImage(backImage, transitionMaskImage: backImage)

        let navigationBar = UINavigationBar.appearance()
        navigationBar.standardAppearance = standard
        navigationBar.compactAppearance = standard
        navigationBar.scrollEdgeAppearance = scrollEdge
        navigationBar.compactScrollEdgeAppearance = scrollEdge
        navigationBar.tintColor = primaryText
        navigationBar.prefersLargeTitles = false

        UITabBar.appearance().tintColor = tint
        if #unavailable(iOS 26) {
            let tabBar = UITabBarAppearance()
            tabBar.configureWithDefaultBackground()
            let tabItem = UITabBarItemAppearance()
            tabItem.normal.iconColor = secondaryText
            tabItem.normal.titleTextAttributes = [.foregroundColor: secondaryText]
            tabItem.selected.iconColor = tint
            tabItem.selected.titleTextAttributes = [.foregroundColor: tint]
            tabBar.stackedLayoutAppearance = tabItem
            tabBar.inlineLayoutAppearance = tabItem
            tabBar.compactInlineLayoutAppearance = tabItem
            UITabBar.appearance().standardAppearance = tabBar
            UITabBar.appearance().scrollEdgeAppearance = tabBar
            UITabBar.appearance().unselectedItemTintColor = secondaryText
        }

        // Legacy Form / List screens until they migrate to MonacoGroupedList.
        UITableView.appearance().backgroundColor = .clear
        UITableView.appearance().separatorColor = border

        // No global UITextField / UISegmentedControl appearance: it leaked into Privy's sheets.
        // Use MonacoTextField and MonacoSegmented instead.
    }
}

struct MonacoRootAppearanceModifier: ViewModifier {
    func body(content: Content) -> some View {
        content
            .monacoCanvas()
            .foregroundStyle(MonacoTheme.primaryText)
    }
}

extension EnvironmentValues {
    @Entry var insideMonacoSheet = false
    @Entry var monacoReduceTransparencyOverride = false
}

@propertyWrapper
struct MonacoReduceTransparency: DynamicProperty {
    @Environment(\.accessibilityReduceTransparency) private var system
    @Environment(\.monacoReduceTransparencyOverride) private var override

    var wrappedValue: Bool { system || override }
}

extension View {
    /// Flat paper canvas. Apply once at a screen root.
    func monacoCanvas() -> some View {
        background { MonacoCanvasBackground() }
    }

    /// Apply at screen root (or rely on `ContentView` / `MonacoApp` wiring).
    func monacoRootAppearance() -> some View {
        modifier(MonacoRootAppearanceModifier())
    }

    @ViewBuilder
    func monacoActionCard(
        isCard: Bool = true,
        fill: Color = MonacoTheme.surface,
        border: Color = MonacoTheme.border,
        isDashed: Bool = false,
        inset: CGFloat = MonacoTheme.Space.m,
        fillsWidth: Bool = true
    ) -> some View {
        if isCard {
            let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
            self
                .padding(inset)
                .frame(maxWidth: fillsWidth ? .infinity : nil, alignment: .leading)
                .background(fill, in: shape)
                .overlay {
                    shape.strokeBorder(border, style: StrokeStyle(lineWidth: 1, dash: isDashed ? [6, 4] : []))
                }
        } else {
            self.frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    func monacoSheet(
        title: String,
        titleIdentifier: String? = nil,
        detents: MonacoSheetDetents? = nil,
        showsClose: Bool = false,
        closeIdentifier: String? = nil
    ) -> some View {
        MonacoSheetFrame(
            title: title, titleIdentifier: titleIdentifier, kind: detents, showsClose: showsClose,
            closeIdentifier: closeIdentifier, scrolls: false, content: self)
    }

    func monacoFittedSheet(
        title: String,
        titleIdentifier: String? = nil,
        detents: MonacoSheetDetents = .fitted,
        showsClose: Bool = false,
        closeIdentifier: String? = nil,
        showsHeading: Bool = true
    ) -> some View {
        MonacoSheetFrame(
            title: title, titleIdentifier: titleIdentifier, kind: detents, showsClose: showsClose,
            closeIdentifier: closeIdentifier, scrolls: true, showsHeading: showsHeading, content: self)
    }

    @ViewBuilder
    func monacoNavHairlineHidden() -> some View {
        if #available(iOS 26, *) {
            self
        } else {
            toolbarBackground(.hidden, for: .navigationBar)
        }
    }

    func monacoTopLevelHeader(title: String) -> some View {
        navigationTitle(title)
            .navigationBarTitleDisplayMode(.large)
    }

    func monacoBleed() -> some View {
        padding(.horizontal, -MonacoTheme.Space.gutter)
    }

    /// Toolbar / nav bar SF Symbol: 17pt semibold, ink, monochrome.
    func monacoToolbarIcon() -> some View {
        font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.ink)
            .symbolRenderingMode(.monochrome)
    }

    /// Form screen root — canvas background, visible rows/separators, readable fields.
    func monacoFormScreen() -> some View {
        scrollContentBackground(.hidden)
            .monacoCanvas()
            .foregroundStyle(MonacoTheme.primaryText)
            .tint(MonacoTheme.accent)
            .listRowBackground(MonacoTheme.surface)
            .listRowSeparatorTint(MonacoTheme.border)
    }

    /// Text field inside a Form section — ink text + accent caret.
    func monacoFormTextField() -> some View {
        foregroundStyle(MonacoTheme.primaryText)
            .tint(MonacoTheme.accent)
    }

    /// Primary action button row inside a Form.
    func monacoFormPrimaryAction() -> some View {
        buttonStyle(.monacoPrimary)
            .frame(maxWidth: .infinity)
            .listRowInsets(EdgeInsets(top: 8, leading: 16, bottom: 8, trailing: 16))
            .listRowBackground(Color.clear)
    }

}

enum MonacoSheetDetents {
    case review, votes, result, fitted, tall

    static let halfFraction: CGFloat = 0.5
    static let tallFraction: CGFloat = 0.72

    static let half: [PresentationDetent] = [.medium]

    @MainActor
    static var screenHeight: CGFloat {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        return scenes.first?.screen.bounds.height ?? 0
    }

    static func scrollsContent(declared: Bool, dynamicTypeSize: DynamicTypeSize) -> Bool {
        declared || dynamicTypeSize.isAccessibilitySize
    }

    @MainActor
    func detents(
        fittedHeight: CGFloat, screenHeight: CGFloat = MonacoSheetDetents.screenHeight,
        dynamicTypeSize: DynamicTypeSize = .large
    ) -> [PresentationDetent] {
        let base: [PresentationDetent]
        switch self {
        case .review, .votes:
            base = Self.half
        case .tall:
            base = [.fraction(Self.tallFraction)]
        case .result, .fitted:
            if fittedHeight > 0, fittedHeight < screenHeight * Self.halfFraction {
                base = [.height(fittedHeight)]
            } else {
                base = Self.half
            }
        }
        return dynamicTypeSize.isAccessibilitySize ? base + [.large] : base
    }
}

struct MonacoSheetHeading: View {
    let title: String
    let identifier: String?
    var showsClose = false
    var closeIdentifier: String?

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            titleText
            if showsClose {
                closeButton
            }
        }
        .frame(minHeight: 48)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.sm)
    }

    @ViewBuilder
    private var closeButton: some View {
        let button = MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet, size: .compact)
        {
            dismiss()
        }
        if let closeIdentifier {
            button.accessibilityIdentifier(closeIdentifier)
        } else {
            button
        }
    }

    @ViewBuilder
    private var titleText: some View {
        let text = Text(title)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(nil)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityAddTraits(.isHeader)
        if let identifier {
            text.accessibilityIdentifier(identifier)
        } else {
            text
        }
    }
}

private struct MonacoSheetFrame<Content: View>: View {
    let title: String
    let titleIdentifier: String?
    let kind: MonacoSheetDetents?
    let showsClose: Bool
    let closeIdentifier: String?
    let scrolls: Bool
    var showsHeading = true
    let content: Content

    @State private var headingHeight: CGFloat = 0
    @State private var contentHeight: CGFloat = 0
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if showsHeading {
                MonacoSheetHeading(
                    title: title, identifier: titleIdentifier, showsClose: showsClose,
                    closeIdentifier: closeIdentifier
                )
                .onGeometryChange(for: CGFloat.self) {
                    $0.size.height
                } action: {
                    headingHeight = $0
                }
            }
            sheetContent
        }
        .monacoSheetChrome()
        .presentationDetents(Set(available))
    }

    @ViewBuilder
    private var sheetContent: some View {
        if MonacoSheetDetents.scrollsContent(declared: scrolls, dynamicTypeSize: dynamicTypeSize) {
            ScrollView {
                content
                    .environment(\.insideMonacoSheet, true)
                    .onGeometryChange(for: CGFloat.self) {
                        $0.size.height
                    } action: {
                        contentHeight = $0
                    }
            }
            .scrollBounceBehavior(.basedOnSize)
        } else {
            content
                .environment(\.insideMonacoSheet, true)
        }
    }

    private var available: [PresentationDetent] {
        let headingReady = !showsHeading || headingHeight > 0
        let fitted = headingReady && contentHeight > 0 ? headingHeight + contentHeight : 0
        return (kind ?? .review).detents(fittedHeight: fitted, dynamicTypeSize: dynamicTypeSize)
    }
}

extension View {
    fileprivate func monacoSheetChrome() -> some View {
        modifier(MonacoSheetChrome())
    }
}

private struct MonacoSheetChrome: ViewModifier {
    func body(content: Content) -> some View {
        content
            .presentationDragIndicator(.visible)
            .presentationContentInteraction(.scrolls)
            .presentationBackgroundInteraction(.disabled)
    }
}
