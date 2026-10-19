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
    @Environment(\.cabalsFilter) private var filter

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(CabalsTab.title)
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: MonacoTheme.Space.s)
                if !accountRestricted {
                    MonacoGlassButton(systemImage: MonacoIcon.plus.symbol, accessibilityLabel: "Create a cabal") {
                        environment.navigator.open(CreateCabalRoute(), in: .cabals)
                    }
                    .accessibilityIdentifier("cabals-new-button")
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
            MonacoSearchField(placeholder: "Find your cabals", text: filter)
                .accessibilityIdentifier("cabals-filter-field")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-header")
    }
}
