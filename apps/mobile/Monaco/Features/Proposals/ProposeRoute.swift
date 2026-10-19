import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

nonisolated struct ProposeRoute: AppRoute {
    let cabalID: String
    var start: ProposeStart?

    @MainActor @ViewBuilder func destination() -> some View {
        switch start {
        case nil:
            ProposeChooserScreen(cabalID: cabalID)
                .analyticsScreen("propose", step: .propose(.proposeOpened))
        case .buy:
            ProposeBuyStockView(cabalID: cabalID)
        case .sell:
            CabalPotModelHost(cabalID: cabalID) { pot in
                ProposeSellView(pot: pot)
            }
        }
    }
}

struct ProposeChooserScreen: View {
    let cabalID: String

    var body: some View {
        ScrollView {
            ProposeChoices(cabalID: cabalID, style: .screen) { start in
                environment.navigator.open(
                    ProposeRoute(cabalID: cabalID, start: start), in: hostMainTab ?? environment.navigator.selectedTab)
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .navigationTitle(ProposeFlowCopy.chooserTitle)
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("propose-chooser")
    }

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
}
