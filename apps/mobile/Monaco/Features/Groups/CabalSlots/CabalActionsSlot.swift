import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalActionsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalActionsLive(cabalID: context.cabalID)
    }
}

struct CabalActionsLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalActionsModel?

    init(cabalID: String, model: CabalActionsModel? = nil) {
        self.cabalID = cabalID
        _model = State(initialValue: model)
    }

    var body: some View {
        CabalActionsRow(model: model) { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
        .task(id: retry.tick) {
            let model = preparedModel()
            await model.load()
            await model.loadUnread()
            await model.observe()
        }
    }

    private func preparedModel() -> CabalActionsModel {
        if let model { return model }
        let created = CabalActionsModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct CabalActionsRow: View {
    let model: CabalActionsModel?
    let open: (any AppRoute) -> Void

    @State private var toast: MonacoToast?

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .monacoToast($toast)
            .onChange(of: model?.toast) { _, message in
                guard let model, let message else { return }
                toast = MonacoToast(message: message)
                model.dismissToast()
            }
    }

    @ViewBuilder private var content: some View {
        switch model?.actions ?? .loading {
        case .loading:
            VStack(spacing: MonacoTheme.Space.gutter) {
                HStack(spacing: 0) {
                    ForEach(0..<4, id: \.self) { _ in
                        SkeletonBlock(width: 56, height: 56, radius: 28)
                            .frame(maxWidth: .infinity)
                    }
                }
                MonacoRowSkeleton(rows: 3, markShape: .tile)
            }
            .accessibilityHidden(true)
        case .hidden, .failed:
            Color.clear.frame(height: 0)
        case .member(let canPropose):
            if let model {
                buttons(cabalID: model.cabalID, canPropose: canPropose)
            }
        }
    }

    private func buttons(cabalID: String, canPropose: Bool) -> some View {
        VStack(spacing: MonacoTheme.Space.s) {
            HStack(alignment: .top, spacing: 0) {
                action("Fund", "plus", id: "cabal-action-fund", FundRoute(cabalID: cabalID))
                action("Propose", "arrow.up.right", id: "cabal-action-propose", ProposeRoute(cabalID: cabalID))
                    .disabled(!canPropose)
                action("Cash out", "arrow.down.left", id: "cabal-action-cash-out", CashOutRoute(cabalID: cabalID))
                action(
                    "Chat", "bubble.left", id: "cabal-action-chat", ChatRoute(cabalID: cabalID),
                    unread: model?.hasUnreadChat == true)
            }
            if !canPropose {
                Text("Only voters can propose")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("cabal-action-propose-caption")
            }
        }
    }

    private func action(
        _ title: String, _ systemImage: String, id: String, _ route: some AppRoute, unread: Bool = false
    ) -> some View {
        CircleAction(title, systemImage: systemImage) { open(route) }
            .frame(maxWidth: .infinity)
            .overlay(alignment: .top) { if unread { UnreadDot() } }
            .accessibilityLabel(unread ? CabalCopy.chatUnreadAction : title)
            .accessibilityIdentifier(id)
    }
}

private struct UnreadDot: View {
    @ScaledMetric(relativeTo: .footnote) private var scaledDiscSize: CGFloat = 56

    var body: some View {
        Circle()
            .fill(MonacoTheme.destructive)
            .frame(width: 8, height: 8)
            .overlay(Circle().strokeBorder(MonacoTheme.background, lineWidth: 1.5))
            .offset(x: CircleActionMetrics.discSize(scaled: scaledDiscSize) / 2 - 6, y: 2)
            .accessibilityHidden(true)
            .accessibilityIdentifier("cabal-action-chat-unread")
    }
}

#if DEBUG
final class CabalActionsSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalActionsHarness") else { return nil }
        let role = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "voter"
        return AnyView(CabalActionsHarnessScreen(role: role))
    }
}

private struct CabalActionsHarnessScreen: View {
    let role: String
    @State private var model: CabalActionsModel?
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            VStack(spacing: 0) {
                CabalActionsRow(model: model) { path.append(AnyAppRoute($0)) }
                Spacer(minLength: 0)
            }
            .padding(.top, MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .navigationTitle("QA pot")
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
            .task {
                let created = model ?? CabalActionsModel.preview(Self.sample(role))
                model = created
                await created.load()
            }
        }
    }

    private static func sample(_ role: String) -> Components.Schemas.Cabal {
        switch role {
        case "nonvoter": .sample(role: "member", canVote: false)
        case "outsider": .sample(role: nil)
        default: .sample(role: "member", canVote: true)
        }
    }
}
#endif
