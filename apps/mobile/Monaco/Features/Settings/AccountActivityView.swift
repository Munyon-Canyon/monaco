import MonacoCore
import SwiftUI

struct AccountActivityView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: AccountActivityModel?

    var body: some View {
        AccountActivityList(model: model)
            .task {
                let model = preparedModel()
                await model.load()
                await model.observe()
            }
    }

    private func preparedModel() -> AccountActivityModel {
        if let model { return model }
        let created = AccountActivityModel(api: environment.api, hints: environment.hints, clock: Date.init)
        model = created
        return created
    }
}

struct AccountActivityList: View {
    let model: AccountActivityModel?

    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        ScrollView {
            content
        }
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle("Account activity")
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: model?.toast) { _, message in
            guard let model, let message else { return }
            toasts.current = MonacoToast(message: message)
            model.dismissToast()
        }
        .onChange(of: model?.rows.count) { _, _ in
            Task { await model?.fillFilter() }
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            skeleton
        case .failed:
            MoneyLoadFailure(identifier: "account-activity-error") {
                Task { await model?.load() }
            }
        case .empty:
            VStack(alignment: .leading, spacing: 0) {
                header
                EmptyState(
                    title: "No transactions yet",
                    message: "Personal deposits, deposits, redemptions and withdrawals will appear here.",
                    systemImage: MonacoIcon.receipt.symbol
                )
                .accessibilityIdentifier("account-activity-empty")
            }
        case .loaded:
            VStack(alignment: .leading, spacing: 0) {
                header
                list
                MonacoNotice(
                    .info,
                    message:
                        "This is your personal account history. Cabal trades are viewed through their cabal and proposal records."
                )
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.m)
                .padding(.bottom, MonacoTheme.Space.l)
            }
        }
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                SkeletonTitle()
                SkeletonBar()
                SkeletonCardBlock()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRowSkeleton(rows: 4, markShape: .circle)
        }
        .padding(.top, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading activity")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text("Account activity")
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityAddTraits(.isHeader)
            MonacoSegmented(
                filterOptions,
                selection: filterSelection,
                identifier: filterIdentifier,
                label: filterLabel
            )
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .padding(.bottom, MonacoTheme.Space.l)
    }

    private var filterOptions: [AccountActivityModel.Filter] { [.all, .pending] }

    private var filterSelection: Binding<AccountActivityModel.Filter> {
        Binding(
            get: { model?.filter ?? .all },
            set: { selected in
                model?.filter = selected
                Task { await model?.fillFilter() }
            })
    }

    private func filterIdentifier(_ filter: AccountActivityModel.Filter) -> String {
        filter == .all ? "account-activity-filter-all" : "account-activity-filter-pending"
    }

    private func filterLabel(_ filter: AccountActivityModel.Filter) -> String {
        filter == .all ? "All" : "Pending"
    }

    @ViewBuilder private var list: some View {
        let rows = model?.visibleRows ?? []
        if model?.isFilterEmpty == true {
            EmptyState(
                title: "No pending transfers",
                message: "Transfers that are still processing show up here.",
                systemImage: MonacoIcon.receipt.symbol
            )
            .accessibilityIdentifier("account-activity-pending-empty")
        } else {
            MonacoGroupedList {
                LazyVStack(spacing: 0) {
                    ForEach(rows) { row in
                        NavigationLink(
                            value: AnyAppRoute(AccountTxnReceiptRoute(row: row))
                        ) {
                            AccountActivityRowView(row: row, isLast: row.id == rows.last?.id)
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("account-activity-row-\(row.id)")
                        .onAppear {
                            guard row.id == rows.last?.id else { return }
                            Task { await model?.loadMore() }
                        }
                    }
                }
                if model?.isLoadingMore == true {
                    MonacoRowSkeleton(rows: 1, markShape: .circle)
                }
            }
        }
    }
}

private struct AccountActivityRowView: View {
    let row: AccountActivityRow
    let isLast: Bool

    var body: some View {
        MonacoRow(title: row.title, subtitle: row.subtitle, chevron: true, isLast: isLast) {
            SunkenGlyphMark(systemImage: row.glyph)
        } trailing: {
            trailing
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spokenLabel)
    }

    @ViewBuilder private var trailing: some View {
        switch row.status {
        case .settled:
            amount
        case .pending:
            StatusBadge("Pending", tone: .warn)
        case .failed:
            StatusBadge("Failed", tone: .negative)
        }
    }

    private var amount: some View {
        Text(row.amount)
            .moneyFigure(.row)
            .foregroundStyle(MonacoTheme.ink)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
    }

    private var spokenLabel: String {
        let status = row.status.rowLabel.map { ", \($0)" } ?? ""
        return "\(row.title), \(row.subtitle), \(row.amount)\(status)"
    }
}

#if DEBUG
final class AccountActivitySampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-accountActivityHarness") else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "full"
        return AnyView(AccountActivityHarnessScreen(mode: mode))
    }
}

private struct AccountActivityHarnessScreen: View {
    let mode: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: AccountActivityModel?
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            AccountActivityList(model: model)
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
                .task {
                    let created = await makeModel()
                    model = created
                    if mode == "pending" { created.filter = .pending }
                    await created.load()
                    await created.fillFilter()
                    await created.observe()
                }
        }
    }

    private func makeModel() async -> AccountActivityModel {
        switch mode {
        case "live":
            if !environment.isSignedIn, let session = DevSession.fromLaunchEnvironment() {
                await environment.signIn(dev: session)
            }
            return AccountActivityModel(api: environment.api, hints: environment.hints, clock: Date.init)
        case "ready": return .preview(.ready, clock: Date.init)
        case "pending": return .preview(.pending, clock: Date.init)
        case "empty": return .preview(.empty, clock: Date.init)
        case "loading": return .preview(.loading, clock: Date.init)
        case "error": return .preview(.error, clock: Date.init)
        default: return .preview(.full, clock: Date.init)
        }
    }
}
#endif
