import MonacoCore
import SwiftUI

enum HomeFirstUseSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFirstUse()
    }
}

private struct HomeFirstUse: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @State private var joining = false

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(spacing: MonacoTheme.Space.s) {
            Image(systemName: MonacoIcon.cabals.symbol)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            Text("No cabals yet")
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityAddTraits(.isHeader)
            Text("A cabal is a few friends, one treasury and a vote on every trade.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
            HStack(spacing: MonacoTheme.Space.s) {
                Button("Start a cabal") {
                    environment.navigator.open(CreateCabalRoute(), in: .home)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("home-first-use-start")
                Button("Join with a link") {
                    joining = true
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("home-first-use-join")
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity)
        .homeSlotHidden(!gate.firstUse || gate.sharedFailure)
        .sheet(isPresented: $joining) { JoinLinkSheet() }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-cabals-empty")
    }
}
