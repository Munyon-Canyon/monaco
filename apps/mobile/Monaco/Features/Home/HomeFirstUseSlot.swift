import MonacoCore
import SwiftUI

enum HomeFirstUseSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFirstUse()
    }
}

private enum FirstUseFaces {
    static let size: CGFloat = 72
    static let ring: CGFloat = 3
    static let overlap: CGFloat = 10
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
                shownCard
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity)
        .homeSlotHidden(hidden)
        .sheet(isPresented: $joining) { JoinLinkSheet() }
    }

    private var shownCard: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-cabals-empty")
    }

    private func face(_ character: MonacoCharacter) -> some View {
        Image(character.imageName)
            .resizable()
            .interpolation(.high)
            .scaledToFill()
            .frame(width: FirstUseFaces.size, height: FirstUseFaces.size)
            .clipShape(Circle())
            .overlay(Circle().strokeBorder(MonacoTheme.canvas, lineWidth: FirstUseFaces.ring))
    }

    @ViewBuilder private var content: some View {
        Group {
            HStack(spacing: -FirstUseFaces.overlap) {
                face(.marcus)
                face(.harper)
            }
            .accessibilityHidden(true)
            Text("There is no place like Monaco.")
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityAddTraits(.isHeader)
            Text("Trade with your friends.")
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
