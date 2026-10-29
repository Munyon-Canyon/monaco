import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalEmptySlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalEmptyCard(cabalID: context.cabalID)
    }
}

struct CabalEmptyCard: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel
    @Environment(\.cabalEmptyModel) private var emptyModel
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var showsInvite = false

    var body: some View {
        if let cabal = cabalModel?.cabal, let pot = potModel?.summary,
            emptyModel?.isOn(cabal: cabal, pot: pot) == true
        {
            card(cabal: cabal, pot: pot)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.sm))
        } else {
            Color.clear.frame(height: 0)
        }
    }

    private func card(cabal: Components.Schemas.Cabal, pot: CabalPotSummary) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(CabalEmptyGate.headline())
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("cabal-empty-title")
                Text(CabalEmptyGate.body(name: cabal.name, pot: pot.potValue))
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("cabal-empty-body")
            }
            buttons(cabal: cabal)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(MonacoTheme.Space.m)
        .background(
            MonacoTheme.discussionWash, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-empty-card")
        .sheet(isPresented: $showsInvite) { CabalInviteSheet(cabal: cabal) }
    }

    private func buttons(cabal: Components.Schemas.Cabal) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            if cabal.me?.canPropose == true {
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
