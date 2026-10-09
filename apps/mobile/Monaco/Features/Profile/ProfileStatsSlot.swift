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
            ProfileStatColumn(
                value: stats.inCabals, detail: nil, label: "In cabals", tone: nil, stacked: stacked
            )
            .accessibilityIdentifier("profile-stat-in-cabals")
            ProfileStatColumn(
                value: stats.returnDollars, detail: stats.returnPercent, label: "Return",
                tone: PnLTone(dollarPnl: stats.returnDollars).color, stacked: stacked
            )
            .accessibilityIdentifier("profile-stat-return")
            ProfileStatColumn(value: stats.cabals, detail: nil, label: stats.cabalsLabel, tone: nil, stacked: stacked)
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

    private var stacked: Bool { Self.isStacked(dynamicTypeSize) }

    static func isStacked(_ size: DynamicTypeSize) -> Bool { size.isAccessibilitySize }

    private func columns(@ViewBuilder _ cells: () -> some View) -> some View {
        VStack(spacing: 0) {
            MonacoRule()
            Group {
                if stacked {
                    VStack(spacing: MonacoTheme.Space.m) { cells() }
                } else {
                    HStack(alignment: .top, spacing: MonacoTheme.Space.s) { cells() }
                }
            }
            .padding(.vertical, MonacoTheme.Space.l)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
        }
    }
}

private struct ProfileStatColumn: View {
    let value: String
    let detail: String?
    let label: String
    let tone: Color?
    let stacked: Bool

    var body: some View {
        if stacked {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.m) {
                labelText
                Spacer(minLength: MonacoTheme.Space.s)
                figures(alignment: .trailing)
            }
            .accessibilityElement(children: .combine)
        } else {
            VStack(spacing: MonacoTheme.Space.xs) {
                figures(alignment: .center)
                labelText
            }
            .frame(maxWidth: .infinity)
            .accessibilityElement(children: .combine)
        }
    }

    private var labelText: some View {
        Text(label)
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(stacked ? .leading : .center)
    }

    private func figures(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 0) {
            Text(value)
                .moneyFont(.row)
                .foregroundStyle(tone ?? MonacoTheme.ink)
                .lineLimit(2)
                .minimumScaleFactor(0.8)
            if let detail {
                Text(detail)
                    .font(MonacoTheme.Typo.moneyCaption)
                    .foregroundStyle(tone ?? MonacoTheme.ink)
                    .lineLimit(1)
            }
        }
        .multilineTextAlignment(alignment == .trailing ? .trailing : .center)
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
