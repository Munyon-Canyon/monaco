import MonacoCore
import SwiftUI

enum CabalsJoinSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsHeaderSection()
    }
}

struct CabalsHeaderSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.accountRestricted) private var accountRestricted
    @State private var joining = false

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(CabalsTab.title)
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityAddTraits(.isHeader)
            Spacer(minLength: MonacoTheme.Space.s)
            if !accountRestricted {
                plusMenu
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-header")
        .sheet(isPresented: $joining) { JoinLinkSheet() }
    }

    private var plusMenu: some View {
        Menu {
            Button {
                environment.navigator.open(CreateCabalRoute(), in: .cabals)
            } label: {
                Label("Start a cabal", systemImage: MonacoIcon.cabals.symbol)
            }
            .accessibilityIdentifier("cabals-menu-start")
            Button {
                joining = true
            } label: {
                Label("Join with a link", systemImage: MonacoIcon.link.symbol)
            }
            .accessibilityIdentifier("cabals-menu-join")
            Button {
                environment.navigator.open(DiscoverCabalsRoute(query: ""), in: .cabals)
            } label: {
                Label("Find a cabal", systemImage: MonacoIcon.search.symbol)
            }
            .accessibilityIdentifier("cabals-menu-find")
        } label: {
            Image(systemName: MonacoIcon.plus.symbol)
                .font(.body.weight(.medium))
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(width: PlusMetrics.diameter, height: PlusMetrics.diameter)
                .background(Circle().fill(MonacoTheme.surfaceSunken))
                .contentShape(Circle())
        }
        .accessibilityLabel("Add a cabal")
        .accessibilityIdentifier("cabals-new-button")
    }

    private enum PlusMetrics {
        static let diameter: CGFloat = 44
    }
}
