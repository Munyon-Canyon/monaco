import MonacoCore
import SwiftUI

struct BalancesView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: BalancesModel?

    init(model: BalancesModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        BalancesContent(model: model)
            .task {
                let model = preparedModel()
                await model.load()
            }
    }

    private func preparedModel() -> BalancesModel {
        if let model { return model }
        let created = BalancesModel(
            portfolio: PortfolioModel(api: environment.api, hints: environment.hints), balance: environment.balance)
        model = created
        return created
    }
}

struct BalancesContent: View {
    let model: BalancesModel?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        ScrollView {
            content
        }
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle("Your money")
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: model?.toast) { _, message in
            guard let model, let message else { return }
            toasts.current = MonacoToast(message: message)
            model.dismissToast()
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            skeleton
        case .failed:
            ScreenLoadError(identifier: "balances-error") { Task { await model?.load() } }
        case .loaded:
            if let model { loaded(model) }
        }
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                SkeletonFraction(0.6) {
                    SkeletonBlock(height: SkeletonMetrics.titleHeight, radius: SkeletonMetrics.titleRadius)
                }
                SkeletonFraction(0.4) { SkeletonBar() }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.l)
            SkeletonCardBlock()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.m)
            MonacoRowSkeleton(rows: 4, markShape: .circle)
                .padding(.top, MonacoTheme.Space.section)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading your money")
        .accessibilityIdentifier("balances-loading")
    }

    private func loaded(_ model: BalancesModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            performance(model)
            stats(model)
            actions
            allocation(model)
            MonacoNotice(
                .info,
                message:
                    "These are your own shares, not the full values of your cabals. Personal returns exclude deposits as investment gains."
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
        }
    }

    private func performance(_ model: BalancesModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Net worth")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)
            Text(model.netWorth)
                .moneyFigure(.hero)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .contentTransition(.numericText())
                .accessibilityIdentifier("balances-net-worth")
            Text("Your cabal shares + cash")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.top, MonacoTheme.Space.s)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .combine)
    }

    private func stats(_ model: BalancesModel) -> some View {
        VStack(spacing: 0) {
            MonacoRule(color: MonacoTheme.hairline)
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.gutter) {
                    stat("In cabals", model.inCabals, identifier: "balances-in-cabals")
                    stat("Cash", model.cash, identifier: "balances-cash")
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    stat("In cabals", model.inCabals, identifier: "balances-in-cabals")
                    stat("Cash", model.cash, identifier: "balances-cash")
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
            MonacoRule(color: MonacoTheme.hairline)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
    }

    private func stat(_ label: String, _ value: String, identifier: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Text(value)
                .moneyFigure(.card)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.card.minimumScaleFactor)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }

    private var actions: some View {
        HStack(alignment: .top, spacing: 0) {
            CircleAction("Deposit", systemImage: MonacoIcon.deposit.symbol) {
                environment.navigator.presentDeposit()
            }
            .frame(maxWidth: .infinity)
            .accessibilityIdentifier("balances-deposit")
            tile("Withdraw", "arrow.up.right", WithdrawRoute(), identifier: "balances-withdraw")
            tile("Activity", MonacoIcon.receipt.symbol, AccountActivityRoute(), identifier: "balances-activity")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .padding(.bottom, MonacoTheme.Space.gutter)
    }

    private func tile(_ title: String, _ systemImage: String, _ route: some AppRoute, identifier: String)
        -> some View
    {
        CircleAction(title, systemImage: systemImage) {
            environment.navigator.open(route, in: .home)
        }
        .frame(maxWidth: .infinity)
        .accessibilityIdentifier(identifier)
    }

    private func allocation(_ model: BalancesModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Your allocation")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            if model.isCashOnly {
                MonacoNotice(.info, message: "No funds are allocated to a cabal yet.")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.s)
                    .accessibilityIdentifier("balances-no-allocation")
            }
            MonacoGroupedList {
                ForEach(model.allocation) { row in
                    cabalRow(row)
                }
                cashRow(model)
            }
        }
        .padding(.top, MonacoTheme.Space.xs)
    }

    private func cabalRow(_ row: BalancesModel.Row) -> some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
            MonacoRow(title: row.name, subtitle: "Your share", chevron: true) {
                CabalMark(groupId: row.id, name: row.name, pictureUrl: row.pictureURL, shape: .circle)
            } trailing: {
                Text(row.value)
                    .moneyFigure(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("balances-cabal-\(row.id)")
    }

    private func cashRow(_ model: BalancesModel) -> some View {
        Button {
            environment.navigator.presentDeposit()
        } label: {
            MonacoRow(title: "Cash", subtitle: "Not allocated to a cabal", chevron: true, isLast: true) {
                SunkenGlyphMark(icon: .wallet)
            } trailing: {
                Text(model.cash)
                    .moneyFigure(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("balances-cash-row")
    }
}

struct ScreenLoadError: View {
    let identifier: String
    let retry: () -> Void

    var body: some View {
        EmptyState(
            title: "Couldn’t load this view",
            message: "Your existing information hasn’t been changed. Try again when your connection is available.",
            actionTitle: "Try again", actionIdentifier: "\(identifier)-retry", systemImage: MonacoIcon.offline.symbol,
            action: retry
        )
        .accessibilityIdentifier(identifier)
    }
}

#if DEBUG
final class BalancesSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-balancesHarness") else { return nil }
        let name = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "ready"
        let scenario: BalancesModel.PreviewScenario
        switch name {
        case "cashOnly": scenario = .cashOnly
        case "loading": scenario = .loading
        case "error": scenario = .failed
        default: scenario = .ready
        }
        return AnyView(
            NavigationStack {
                BalancesView(model: .preview(scenario))
                    .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
            }
        )
    }
}
#endif
