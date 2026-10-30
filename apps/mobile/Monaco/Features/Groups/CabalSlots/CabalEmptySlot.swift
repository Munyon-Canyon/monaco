import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalEmptySlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalEmptyCard(cabalID: context.cabalID)
    }
}

struct CabalEmptyReader<Content: View>: View {
    @ViewBuilder let content: (Bool) -> Content

    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel
    @Environment(\.cabalEmptyModel) private var emptyModel

    var body: some View {
        content(
            CabalEmptyGate.isEmpty(
                cabal: cabalModel?.cabal, hasClosedProposal: emptyModel?.hasClosedProposal,
                potState: potModel?.summary?.state))
    }
}

enum CabalEmptyTint {
    static let washOpacity = 0.16
    static let wash = MonacoTheme.CabalTint.teal.fill.opacity(washOpacity)
}

struct CabalEmptyCard: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var showsInvite = false

    var body: some View {
        CabalEmptyReader { isEmpty in
            if isEmpty, let cabal = cabalModel?.cabal, let pot = potModel?.summary {
                card(cabal: cabal, potValue: pot.potValue)
            } else {
                Color.clear.frame(height: 0)
            }
        }
    }

    private func card(cabal: Components.Schemas.Cabal, potValue: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(CabalEmptyGate.title)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            Text(CabalEmptyGate.sentence(cabalName: cabal.name, potValue: potValue))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            buttons(canPropose: CabalEmptyGate.showsPropose(canPropose: cabal.me?.canPropose))
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(MonacoTheme.Space.cardPadding)
        .background(CabalEmptyTint.wash, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.sm))
        .accessibilityIdentifier("cabal-empty-card")
        .sheet(isPresented: $showsInvite) { CabalInviteSheet(cabal: cabal) }
    }

    private func buttons(canPropose: Bool) -> some View {
        let layout =
            dynamicTypeSize >= .xxxLarge
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            if canPropose {
                Button {
                    environment.navigator.open(
                        ProposeRoute(cabalID: cabalID, start: .buy),
                        in: hostMainTab ?? environment.navigator.selectedTab)
                } label: {
                    Label("Propose buy", systemImage: MonacoIcon.propose.symbol)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("cabal-empty-propose")
            }
            Button {
                showsInvite = true
            } label: {
                Label("Invite", systemImage: MonacoIcon.invite.symbol)
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("cabal-empty-invite")
        }
    }
}
