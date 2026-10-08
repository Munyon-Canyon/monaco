import SwiftUI
import UIKit

/// Root appearance wiring and reusable styles for sibling screen agents.
enum MonacoAppearance {
    static func configureUIKit() {
        let canvas = UIColor(MonacoTheme.canvas)
        let surface = UIColor(MonacoTheme.surface)
        let primaryText = UIColor(MonacoTheme.primaryText)
        let muted = UIColor(MonacoTheme.muted)
        let hairline = UIColor(MonacoTheme.hairline)
        let titleFont = UIFont(name: "AvenirNext-DemiBold", size: 17) ?? .systemFont(ofSize: 17, weight: .semibold)
        let largeTitleFont = UIFont(name: "AvenirNext-Bold", size: 32) ?? .systemFont(ofSize: 32, weight: .bold)
        let titleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .headline).scaledFont(for: titleFont, maximumPointSize: 22),
        ]
        let largeTitleAttributes: [NSAttributedString.Key: Any] = [
            .foregroundColor: primaryText,
            .font: UIFontMetrics(forTextStyle: .largeTitle).scaledFont(for: largeTitleFont, maximumPointSize: 44),
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
        standard.shadowColor = hairline
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
        navigationBar.prefersLargeTitles = true

        // Tab bar: opaque surface, hairline top edge.
        let tabBar = UITabBarAppearance()
        tabBar.configureWithOpaqueBackground()
        tabBar.backgroundColor = surface
        tabBar.shadowColor = hairline
        let brand = UIColor(MonacoTheme.brand)
        let tabItem = UITabBarItemAppearance()
        let tabFont = UIFont(name: "AvenirNext-DemiBold", size: 10) ?? .systemFont(ofSize: 10, weight: .semibold)
        tabItem.normal.iconColor = muted
        tabItem.normal.titleTextAttributes = [.foregroundColor: muted, .font: tabFont]
        tabItem.selected.iconColor = brand
        tabItem.selected.titleTextAttributes = [.foregroundColor: brand, .font: tabFont]

        tabBar.stackedLayoutAppearance = tabItem
        tabBar.inlineLayoutAppearance = tabItem
        tabBar.compactInlineLayoutAppearance = tabItem
        UITabBar.appearance().standardAppearance = tabBar
        UITabBar.appearance().scrollEdgeAppearance = tabBar
        UITabBar.appearance().tintColor = brand
        UITabBar.appearance().unselectedItemTintColor = muted

        // Legacy Form / List screens until they migrate to MonacoGroupedList.
        UITableView.appearance().backgroundColor = .clear
        UITableView.appearance().separatorColor = hairline

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

extension View {
    /// Flat paper canvas. Apply once at a screen root.
    func monacoCanvas() -> some View {
        background { MonacoCanvasBackground() }
    }

    /// Apply at screen root (or rely on `ContentView` / `MonacoApp` wiring).
    func monacoRootAppearance() -> some View {
        modifier(MonacoRootAppearanceModifier())
    }

    /// Card-style container on the app canvas.
    func monacoSurfaceCard() -> some View {
        self
            .padding(MonacoTheme.Space.m)
            .background(
                MonacoTheme.surface,
                in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                    .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
            }
    }

    func monacoTopLevelHeader(title: String) -> some View {
        navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(.visible, for: .navigationBar)
    }

    /// Toolbar / nav bar SF Symbol — ink tint, readable weight.
    func monacoToolbarIcon() -> some View {
        font(MonacoTheme.Typo.bodyStrong)
            .foregroundStyle(MonacoTheme.ink)
            .symbolRenderingMode(.hierarchical)
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
