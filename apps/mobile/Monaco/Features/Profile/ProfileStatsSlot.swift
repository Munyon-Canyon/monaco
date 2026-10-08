import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileStatsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileStatsBand()
    }
}

struct ProfileStatsBand: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: PortfolioModel?

    init(model: PortfolioModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        content
            .task {
                let model = preparedModel()
                refresh?.register("profile-stats") { await model.load() }
                await withTaskGroup(of: Void.self) { group in
                    group.addTask { await model.load() }
                    group.addTask { await model.observe() }
                }
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .onChange(of: model?.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message)
                model?.dismissToast()
            }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            skeleton
        case .failed:
            MonacoErrorRow(thing: "your stats", identifier: "profile-stats-retry") {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            band(summary.stats)
        }
    }

    private func band(_ stats: ProfileStats) -> some View {
        columns {
            ProfileStatColumn(value: stats.inCabals, label: "In cabals", tone: nil)
                .accessibilityIdentifier("profile-stat-in-cabals")
            ProfileStatColumn(
                value: stats.allTime, label: "All time", tone: PnLTone(dollarPnl: stats.allTimePnl).color
            )
            .accessibilityIdentifier("profile-stat-all-time")
            ProfileStatColumn(value: stats.cabals, label: "Cabals", tone: nil)
                .accessibilityIdentifier("profile-stat-cabals")
        }
    }

    private var skeleton: some View {
        columns {
            ForEach(0..<3, id: \.self) { _ in
                VStack(spacing: MonacoTheme.Space.xs) {
                    SkeletonBlock(width: 48, height: 16)
                    SkeletonBlock(width: 64, height: 12)
                }
                .frame(maxWidth: .infinity)
            }
        }
        .accessibilityElement()
        .accessibilityLabel("Loading your stats")
        .accessibilityIdentifier("profile-stats-loading")
    }

    private func columns(@ViewBuilder _ cells: () -> some View) -> some View {
        VStack(spacing: 0) {
            MonacoRule()
            HStack(alignment: .top, spacing: 0) { cells() }
                .padding(.vertical, MonacoTheme.Space.m)
            MonacoRule()
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    private func preparedModel() -> PortfolioModel {
        if let model { return model }
        let created = PortfolioModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

private struct ProfileStatColumn: View {
    let value: String
    let label: String
    let tone: Color?

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Text(value)
                .font(MonacoTheme.Typo.dataStrong)
                .foregroundStyle(tone ?? MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .minimumScaleFactor(0.7)
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
    }
}

#if DEBUG
final class ProfilePortfolioHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-profilePortfolioHarness"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        switch arguments[flag + 1] {
        case "two": return AnyView(ProfilePortfolioHarness(portfolio: .sample, auth: auth))
        case "none": return AnyView(ProfilePortfolioHarness(portfolio: .sampleEmpty, auth: auth))
        default: return nil
        }
    }
}

private struct ProfilePortfolioHarness: View {
    let portfolio: Components.Schemas.MyPortfolio
    @State private var environment: AppEnvironment
    @State private var toasts = ToastCenter()

    init(portfolio: Components.Schemas.MyPortfolio, auth: PrivyAuthService) {
        self.portfolio = portfolio
        _environment = State(
            initialValue: AppEnvironment(
                auth: auth, hints: SilentHints(), isAuthenticated: { true }, endAuthSession: {}))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    ProfileStatsBand(model: .preview(portfolio))
                    ProfileCabals(model: .preview(portfolio))
                }
            }
            .monacoCanvas()
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .environment(environment)
        .environment(toasts)
    }
}

private nonisolated struct SilentHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
    func start() async {}
    func stop() async {}
}
#endif
