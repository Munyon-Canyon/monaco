import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct CabalInfoRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CabalInfoView(cabalID: cabalID)
    }
}

struct CabalInfoView: View {
    let cabalID: String
    var savingPreview = false

    @Environment(AppEnvironment.self) private var environment
    @State private var models = CabalScreenModels()
    @State private var retryTick = 0
    @State private var showsInvite = false

    var body: some View {
        let (cabal, pot) = models.prepared(cabalID: cabalID, environment: environment)
        let edit = models.preparedEdit(cabalID: cabalID, environment: environment)
        Group {
            if cabal.cabal == nil, case .failed = cabal.state {
                CabalFullScreenError(identifier: "cabal-info-failed") { retryTick += 1 }
            } else {
                SectionStack(
                    context: CabalContext(cabalID: cabalID), sections: CabalScreen.detailsSections.map { $0.erased }
                )
                .environment(\.cabalModel, cabal)
                .environment(\.cabalPotModel, pot)
                .environment(\.cabalEditModel, edit)
                .environment(\.cabalInfoSaving, savingPreview || edit.isSaving)
                .environment(\.cabalShare, CabalShare { showsInvite = true })
                .environment(\.cabalRetry, CabalRetry(tick: retryTick) { retryTick += 1 })
            }
        }
        .navigationTitle("Cabal information")
        .navigationBarTitleDisplayMode(.inline)
        .monacoCanvas()
        .task(id: retryTick) {
            async let loadedCabal: Void = cabal.load()
            async let loadedPot: Void = pot.load()
            _ = await (loadedCabal, loadedPot)
            async let observedCabal: Void = cabal.observe()
            async let observedPot: Void = pot.observe()
            _ = await (observedCabal, observedPot)
        }
        .onScreenVisibilityChange { visible in
            cabal.setVisible(visible)
            pot.setVisible(visible)
        }
        .sheet(isPresented: $showsInvite) {
            if let shared = cabal.cabal {
                CabalInviteSheet(cabal: shared)
            }
        }
    }
}

enum CabalInfoCoverSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInfoCover()
    }
}

private struct CabalInfoCover: View {
    @Environment(\.cabalModel) private var model

    var body: some View {
        Group {
            switch model?.state ?? .loading {
            case .idle, .loading:
                SkeletonBlock(height: CabalHeroMetrics.skeletonHeight, radius: MonacoTheme.Radius.identity)
                    .accessibilityHidden(true)
            case .failed:
                Color.clear.frame(height: 0)
            case .loaded(let cabal):
                CabalHeroCover(cabal: cabal, subtitle: CabalCopy.roleLine(isCreator: cabal.me?.role == "creator"))
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityIdentifier("cabal-info-cover")
    }
}

enum CabalInfoLinksSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInfoLinks(cabalID: context.cabalID)
    }
}

private struct CabalInfoLinks: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalShare) private var share
    @State private var access: CabalAccessModel?
    @State private var showsInvite = false

    private var requestCount: Int {
        if case .pending(let requests)? = access?.standing { return requests.count }
        return 0
    }

    var body: some View {
        Group {
            if let cabal = cabalModel?.cabal, cabal.me != nil {
                links(cabal)
            } else {
                Color.clear.frame(height: 0)
            }
        }
        .task(id: cabalModel?.cabal) {
            guard let cabal = cabalModel?.cabal, cabal.me?.role == "creator" else { return }
            let model =
                access ?? CabalAccessModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            access = model
            await model.load(cabal: cabal)
        }
        .sheet(isPresented: $showsInvite) {
            if let cabal = cabalModel?.cabal { CabalInviteSheet(cabal: cabal) }
        }
    }

    private func links(_ cabal: Components.Schemas.Cabal) -> some View {
        MonacoGroupedList {
            NavigationLink(value: AnyAppRoute(MembersRoute(cabalID: cabalID, tab: .members))) {
                MonacoRow(
                    title: "Members & voters", subtitle: CabalCopy.memberCount(cabal.memberCount), chevron: true,
                    isLast: false
                ) {
                    SunkenGlyphMark(systemImage: "person.2")
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("cabal-members-row")
            Button {
                if let share { share.open() } else { showsInvite = true }
            } label: {
                MonacoRow(
                    title: "Invite to this cabal", subtitle: "Share an invitation", chevron: true,
                    isLast: requestCount == 0
                ) {
                    SunkenGlyphMark(icon: .share)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("cabal-invite-member-row")
            if requestCount > 0 {
                NavigationLink(value: AnyAppRoute(MembersRoute(cabalID: cabalID, tab: .requests))) {
                    MonacoRow(
                        title: "Join requests", subtitle: CabalCopy.requestsLine(requestCount), chevron: true,
                        isLast: true
                    ) {
                        SunkenGlyphMark(systemImage: "person")
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("cabal-info-requests-row")
            }
        }
    }
}

enum CabalInfoFooterSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInfoFooter(cabalID: context.cabalID)
    }
}

private struct CabalInfoFooter: View {
    let cabalID: String

    @Environment(\.cabalPotModel) private var pot
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalInfoSaving) private var saving

    private var hasShare: Bool {
        guard cabalModel?.cabal?.me != nil, case .stake? = pot?.summary?.slice else { return false }
        return true
    }

    var body: some View {
        if saving || hasShare {
            VStack(spacing: MonacoTheme.Space.sm) {
                if saving {
                    MonacoNotice(.info, message: "Saving your changes\u{2026}")
                        .accessibilityIdentifier("cabal-info-saving")
                }
                if hasShare {
                    NavigationLink(value: AnyAppRoute(CashOutRoute(cabalID: cabalID))) {
                        Text("View your share")
                    }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("cabal-info-view-share")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        } else {
            Color.clear.frame(height: 0)
        }
    }
}
