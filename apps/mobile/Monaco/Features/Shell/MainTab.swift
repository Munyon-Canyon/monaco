import SwiftUI

protocol TabContent {
    associatedtype Root: View
    static var title: String { get }
    static var systemImage: String { get }
    static var accessibilityIdentifier: String { get }
    @MainActor @ViewBuilder static func root() -> Root
}

enum MainTab: Hashable, CaseIterable, Identifiable {
    case home, feed, cabals, stocks, profile

    var id: Self { self }

    var title: String {
        switch self {
        case .home: HomeTab.title
        case .feed: FeedTab.title
        case .cabals: CabalsTab.title
        case .stocks: StocksTab.title
        case .profile: ProfileTab.title
        }
    }

    var systemImage: String {
        switch self {
        case .home: HomeTab.systemImage
        case .feed: FeedTab.systemImage
        case .cabals: CabalsTab.systemImage
        case .stocks: StocksTab.systemImage
        case .profile: ProfileTab.systemImage
        }
    }

    var accessibilityIdentifier: String {
        switch self {
        case .home: HomeTab.accessibilityIdentifier
        case .feed: FeedTab.accessibilityIdentifier
        case .cabals: CabalsTab.accessibilityIdentifier
        case .stocks: StocksTab.accessibilityIdentifier
        case .profile: ProfileTab.accessibilityIdentifier
        }
    }

    @ViewBuilder var root: some View {
        switch self {
        case .home: HomeTab.root()
        case .feed: FeedTab.root()
        case .cabals: CabalsTab.root()
        case .stocks: StocksTab.root()
        case .profile: ProfileTab.root()
        }
    }
}
