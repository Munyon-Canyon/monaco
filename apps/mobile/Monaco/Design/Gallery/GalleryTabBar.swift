#if DEBUG
import SwiftUI

enum GalleryTab: String, CaseIterable, Identifiable {
    case tokens, type, controls, chrome, surfaces, marks, charts, cards, money, toast, shareCard

    var id: String { rawValue }

    var title: String {
        switch self {
        case .tokens: "Tokens"
        case .type: "Type"
        case .controls: "Controls"
        case .chrome: "Chrome"
        case .surfaces: "Surfaces"
        case .marks: "Marks"
        case .charts: "Charts"
        case .cards: "Cards"
        case .money: "Money"
        case .toast: "Toast"
        case .shareCard: "Share card"
        }
    }

    var wrapsInNavigation: Bool { self != .chrome }

    init?(launchValue: String) {
        switch launchValue {
        case "primitives": self = .tokens
        case "balance": self = .cards
        default: self.init(rawValue: launchValue)
        }
    }

    @ViewBuilder
    var page: some View {
        switch self {
        case .tokens: GalleryTokensPage()
        case .type: GalleryTypePage()
        case .controls: GalleryControlsPage()
        case .chrome: GalleryChromePage()
        case .surfaces: GallerySurfacesPage()
        case .marks: GalleryMarksPage()
        case .charts: GalleryChartsPage()
        case .cards: GalleryCardsPage()
        case .money: GalleryMoneyPage()
        case .toast: GalleryToastPage()
        case .shareCard: GalleryShareCardPage()
        }
    }
}

struct GalleryTabBar: View {
    @Binding var selection: GalleryTab

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: MonacoTheme.Space.s) {
                    ForEach(GalleryTab.allCases) { tab in
                        Button {
                            selection = tab
                        } label: {
                            Text(tab.title).monacoChipLabel(isSelected: tab == selection)
                        }
                        .buttonStyle(.plain)
                        .id(tab)
                        .accessibilityAddTraits(tab == selection ? [.isSelected] : [])
                        .accessibilityIdentifier("gallery-tab-\(tab.rawValue)")
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            .onAppear { proxy.scrollTo(selection, anchor: .center) }
        }
        .padding(.vertical, MonacoTheme.Space.xs)
        .frame(maxWidth: .infinity)
        .background(MonacoTheme.canvas)
        .overlay(alignment: .top) { MonacoRule() }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Gallery pages")
    }
}

struct GalleryTabBarSample: View {
    @State private var selection: MainTab = .home

    var body: some View {
        TabView(selection: $selection) {
            ForEach(MainTab.allCases) { tab in
                MonacoTheme.canvas
                    .tabItem {
                        Label(tab.title, systemImage: tab.systemImage)
                            .environment(\.symbolVariants, tab == selection ? .fill : .none)
                    }
                    .badge(tab == .cabals ? 1 : 0)
                    .tag(tab)
            }
        }
        .tint(MonacoTheme.tint)
        .frame(height: 140)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1))
    }
}
#endif
