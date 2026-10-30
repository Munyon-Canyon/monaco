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
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var joining = false

    private var actionsLayout: AnyLayout {
        dynamicTypeSize >= .xxLarge
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
    }

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        let hidden = !gate.firstUse || gate.sharedFailure
        VStack(spacing: MonacoTheme.Space.s) {
            if !hidden {
                content
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity)
        .homeSlotHidden(hidden)
        .sheet(isPresented: $joining) { JoinLinkSheet() }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-cabals-empty")
    }

    @ViewBuilder private var content: some View {
        Group {
            Image(systemName: MonacoIcon.pair.symbol)
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
                .fixedSize(horizontal: false, vertical: true)
            actionsLayout {
                Button {
                    environment.navigator.open(CreateCabalRoute(), in: .home)
                } label: {
                    Label("Start a cabal", systemImage: MonacoIcon.plus.symbol)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("home-first-use-start")
                Button {
                    joining = true
                } label: {
                    Label("Join with a link", systemImage: MonacoIcon.link.symbol)
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("home-first-use-join")
            }
            .padding(.top, MonacoTheme.Space.s)
        }
    }
}
