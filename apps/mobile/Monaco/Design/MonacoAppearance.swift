import SwiftUI
import UIKit

/// Root appearance wiring and reusable styles for sibling screen agents.
enum MonacoAppearance {
    static func configureUIKit() {
        let canvas = UIColor(MonacoTheme.canvas)
        let primaryText = UIColor(MonacoTheme.primaryText)
        let secondaryText = UIColor(MonacoTheme.secondaryText)
        let brand = UIColor(MonacoTheme.brand)
        let border = UIColor(MonacoTheme.border)
        let titleFont = UIFont.systemFont(ofSize: 17, weight: .semibold)
        let largeTitleFont = UIFont.systemFont(ofSize: 30, weight: .bold)
        let titleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .headline).scaledFont(for: titleFont, maximumPointSize: 22),
        ]
        let largeTitleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .largeTitle).scaledFont(for: largeTitleFont, maximumPointSize: 36),
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

        // Scrolled: opaque canvas with a hairline, so content never slides under the title.
        let standard = UINavigationBarAppearance()
        standard.configureWithOpaqueBackground()
        standard.backgroundColor = canvas
        standard.shadowColor = border
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

        // Tab bar: the system material, which goes opaque under Reduce Transparency; the selected item in brand.
        let tabBar = UITabBarAppearance()
        tabBar.configureWithDefaultBackground()
        tabBar.shadowColor = border
        let tabItem = UITabBarItemAppearance()
        let tabFont = UIFontMetrics(forTextStyle: .caption2)
            .scaledFont(for: .systemFont(ofSize: 11, weight: .medium), maximumPointSize: 15)
        tabItem.normal.iconColor = secondaryText
        tabItem.normal.titleTextAttributes = [.foregroundColor: secondaryText, .font: tabFont]
        tabItem.selected.iconColor = brand
        tabItem.selected.titleTextAttributes = [.foregroundColor: brand, .font: tabFont]

        tabBar.stackedLayoutAppearance = tabItem
        tabBar.inlineLayoutAppearance = tabItem
        tabBar.compactInlineLayoutAppearance = tabItem
        UITabBar.appearance().standardAppearance = tabBar
        UITabBar.appearance().scrollEdgeAppearance = tabBar
        UITabBar.appearance().tintColor = brand
        UITabBar.appearance().unselectedItemTintColor = secondaryText

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
        showsClose: Bool = false
    ) -> some View {
        MonacoSheetFrame(
            title: title, titleIdentifier: titleIdentifier, kind: detents, showsClose: showsClose, scrolls: false,
            content: self)
    }

    func monacoFittedSheet(
        title: String,
        titleIdentifier: String? = nil,
        detents: MonacoSheetDetents = .fitted,
        showsClose: Bool = false
    ) -> some View {
        MonacoSheetFrame(
            title: title, titleIdentifier: titleIdentifier, kind: detents, showsClose: showsClose, scrolls: true,
            content: self)
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
    case review, votes, result, fitted

    static let halfFraction: CGFloat = 0.55
    static let resultHeight: CGFloat = 310

    func detents(fittedHeight: CGFloat) -> [PresentationDetent] {
        switch self {
        case .review, .votes: return [.fraction(Self.halfFraction)]
        case .result: return [.height(Self.resultHeight)]
        case .fitted: return fittedHeight > 0 ? [.height(fittedHeight)] : [.fraction(Self.halfFraction)]
        }
    }
}

struct MonacoSheetHeading: View {
    let title: String
    let identifier: String?
    var showsClose = false

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            titleText
            if showsClose {
                MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet) { dismiss() }
            }
        }
        .frame(minHeight: 48)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.sm)
    }

    @ViewBuilder
    private var titleText: some View {
        let text = Text(title)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
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
    let scrolls: Bool
    let content: Content

    @State private var headingHeight: CGFloat = 0
    @State private var contentHeight: CGFloat = 0

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSheetHeading(title: title, identifier: titleIdentifier, showsClose: showsClose)
                .onGeometryChange(for: CGFloat.self) {
                    $0.size.height
                } action: {
                    headingHeight = $0
                }
            sheetContent
        }
        .monacoSheetChrome()
        .presentationDetents(Set(available))
    }

    @ViewBuilder
    private var sheetContent: some View {
        if scrolls {
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
        guard let kind else { return [.large] }
        return kind.detents(fittedHeight: headingHeight > 0 && contentHeight > 0 ? headingHeight + contentHeight : 0)
    }
}

extension View {
    fileprivate func monacoSheetChrome() -> some View {
        modifier(MonacoSheetChrome())
    }
}

private struct MonacoSheetChrome: ViewModifier {
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    @ViewBuilder
    func body(content: Content) -> some View {
        let base =
            content
            .presentationDragIndicator(.visible)
            .presentationContentInteraction(.scrolls)
            .presentationBackgroundInteraction(.disabled)
        if reduceTransparency {
            base.presentationBackground(MonacoTheme.background)
        } else if #available(iOS 26, *) {
            base
        } else {
            base.presentationBackground(.regularMaterial)
        }
    }
}
