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
    @Environment(\.cabalModel) private var cabalModel
    @State private var model: CabalActionsModel?

    init(cabalID: String, model: CabalActionsModel? = nil) {
        self.cabalID = cabalID
        _model = State(initialValue: model)
    }

    var body: some View {
        CabalActionsRow(model: model, retry: { retry.retry?() }) { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
        .onChange(of: cabalModel?.cabal, initial: true) { _, cabal in
            guard let cabal else { return }
            preparedModel().apply(cabal)
        }
        .onChange(of: cabalModel?.failureTick) { _, _ in
            guard cabalModel?.cabal == nil, let error = cabalModel?.lastError else { return }
            preparedModel().fail(error)
        }
        .task(id: retry.tick) {
            let model = preparedModel()
            guard cabalModel == nil else { return }
            await model.load()
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
    let retry: () -> Void
    let open: (any AppRoute) -> Void

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel
    @Environment(\.cabalEmptyModel) private var emptyModel
    @Environment(\.accountRestricted) private var accountRestricted
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var showsProposeChooser = false

    private var hasNoShare: Bool { potModel?.summary?.slice == CabalPotSummary.Slice.none }

    var body: some View {
        content
            .onChange(of: model?.toast) { _, message in
                guard let model, let message else { return }
                toasts.current = MonacoToast(message: message)
                model.dismissToast()
            }
    }

    @ViewBuilder private var content: some View {
        if cabalModel?.cabal?.status == CabalClosedModel.closedStatus
            || emptyModel?.isOn(cabal: cabalModel?.cabal, pot: potModel?.summary) == true
        {
            Color.clear.frame(height: 0)
        } else {
            actionsContent
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.sm))
        }
    }

    @ViewBuilder private var actionsContent: some View {
        switch model?.actions ?? .loading {
        case .loading:
            if cabalModel?.cabal?.me == nil, cabalModel?.cabal != nil {
                Color.clear.frame(height: 0)
            } else {
                SkeletonBlock(height: MonacoButtonMetrics.minimumHeight, radius: MonacoTheme.Radius.button)
                    .accessibilityHidden(true)
            }
        case .hidden:
            Color.clear.frame(height: 0)
        case .failed:
            MonacoErrorRow(
                thing: "the cabal actions", identifier: "cabal-actions-failed", inset: false, retry: retry)
        case .member(let canVote):
            if let model {
                buttons(cabalID: model.cabalID, canPropose: cabalModel?.cabal?.me?.canPropose ?? canVote)
            }
        }
    }

    private func buttons(cabalID: String, canPropose: Bool) -> some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            pair(cabalID: cabalID, canPropose: canPropose)
            if !canPropose {
                Text(CabalCopy.onlyVotersCaption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("cabal-action-propose-caption")
            }
            if hasNoShare {
                MonacoNotice(.info, message: CabalCopy.noShareNotice)
                    .accessibilityIdentifier("cabal-no-share-notice")
            }
        }
    }

    private func pair(cabalID: String, canPropose: Bool) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            if !accountRestricted {
                Button {
                    open(FundRoute(cabalID: cabalID))
                } label: {
                    Label("Fund", systemImage: MonacoIcon.deposit.symbol)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityLabel(CabalCopy.fundTerm)
                .accessibilityIdentifier("cabal-action-fund")
            }
            if canPropose {
                Button {
                    showsProposeChooser = true
                } label: {
                    Label("Propose", systemImage: MonacoIcon.propose.symbol)
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("cabal-action-propose")
                .proposeChooser(isPresented: $showsProposeChooser, cabalID: cabalID)
            }
        }
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
                CabalActionsRow(model: model, retry: { Task { await model?.load() } }) {
                    path.append(AnyAppRoute($0))
                }
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
