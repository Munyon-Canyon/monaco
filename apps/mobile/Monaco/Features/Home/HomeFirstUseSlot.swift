import MonacoCore
import SwiftUI

enum HomeFirstUseSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFirstUse()
    }
}

enum HomeFirstUseMetrics {
    static let glyph: CGFloat = 40
    static let messageWidth: CGFloat = 320
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
                .font(MonacoTheme.Typo.scaled(size: HomeFirstUseMetrics.glyph, weight: .regular, relativeTo: .title2))
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            Text("No cabals yet")
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            Text("A cabal is a few friends, one treasury and a vote on every trade.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: HomeFirstUseMetrics.messageWidth)
            buttons
                .padding(.top, MonacoTheme.Space.m)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity)
        .homeSlotHidden(!gate.firstUse)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-first-use")
        .sheet(isPresented: $joining) { JoinLinkSheet() }
    }

    @ViewBuilder private var buttons: some View {
        let layout = ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.sm) {
                start
                join
            }
            VStack(spacing: MonacoTheme.Space.sm) {
                start
                join
            }
        }
        layout
    }

    private var start: some View {
        Button {
            environment.navigator.open(CreateCabalRoute(), in: .home)
        } label: {
            Label("Start a cabal", systemImage: MonacoIcon.plus.symbol)
        }
        .buttonStyle(.monacoPrimary)
        .accessibilityIdentifier("home-first-use-start")
    }

    private var join: some View {
        Button {
            joining = true
        } label: {
            Label("Join with a link", systemImage: MonacoIcon.link.symbol)
        }
        .buttonStyle(.monacoSecondary)
        .accessibilityIdentifier("home-first-use-join")
    }
}
