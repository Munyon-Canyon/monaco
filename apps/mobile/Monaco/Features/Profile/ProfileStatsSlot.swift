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
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        content
    }

    @ViewBuilder private var content: some View {
        switch portfolio?.state ?? .loading {
        case .idle, .loading:
            skeleton
        case .failed:
            MonacoErrorRow(thing: "your portfolio", identifier: "profile-stats-retry") {
                Task { await portfolio?.load() }
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
            Group {
                if dynamicTypeSize.isAccessibilitySize {
                    VStack(spacing: MonacoTheme.Space.m) { cells() }
                } else {
                    HStack(alignment: .top, spacing: MonacoTheme.Space.s) { cells() }
                }
            }
            .padding(.vertical, MonacoTheme.Space.l)
            MonacoRule()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

private struct ProfileStatColumn: View {
    let value: String
    let label: String
    let tone: Color?

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Text(value)
                .font(MonacoTheme.Typo.moneyRow)
                .foregroundStyle(tone ?? MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .minimumScaleFactor(0.8)
            Text(label)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
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
    @State private var model: PortfolioModel

    init(portfolio: Components.Schemas.MyPortfolio, auth: PrivyAuthService) {
        self.portfolio = portfolio
        _model = State(initialValue: .preview(portfolio))
        _environment = State(
            initialValue: AppEnvironment(
                auth: auth, hints: SilentHints(), isAuthenticated: { true }, endAuthSession: {}))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    ProfileStatsBand()
                    ProfileCabals()
                }
            }
            .monacoCanvas()
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .environment(environment)
        .environment(toasts)
        .environment(model)
        .task { await model.load() }
    }
}

private nonisolated struct SilentHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
    func start() async {}
    func stop() async {}
}
#endif
