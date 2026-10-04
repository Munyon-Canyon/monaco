import MonacoCore
import SwiftUI

struct AccountActivityView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: AccountActivityModel?

    var body: some View {
        AccountActivityList(model: model) { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
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
    let open: (any AppRoute) -> Void

    @Environment(ToastCenter.self) private var toasts
    @State private var receipt: AccountActivityRow?
    @State private var cabalToOpen: String?

    var body: some View {
        ScrollView {
            content
        }
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle("Activity")
        .navigationBarTitleDisplayMode(.inline)
        .onChange(of: model?.toast) { _, message in
            guard let model, let message else { return }
            toasts.current = MonacoToast(message: message)
            model.dismissToast()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .sheet(item: $receipt, onDismiss: openPendingCabal) { row in
            AccountTxnReceiptSheet(row: row) { cabalID in
                cabalToOpen = cabalID
                receipt = nil
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            BoardRowSkeleton(rows: 6)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading activity")
        case .empty:
            EmptyState(
                title: "No activity yet", message: "Deposits, withdrawals and cabal moves show up here.",
                actionTitle: nil
            )
            .accessibilityIdentifier("account-activity-empty")
        case .failed:
            EmptyState(title: "Couldn't load your activity.", actionTitle: "Try again") {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("account-activity-error")
        case .loaded:
            list
        }
    }

    private var list: some View {
        let rows = model?.rows ?? []
        let loadingMore = model?.isLoadingMore == true
        return MonacoGroupedList(rules: loadingMore ? .top : .both) {
            LazyVStack(spacing: 0) {
                ForEach(rows) { row in
                    Button {
                        receipt = row
                    } label: {
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
            if loadingMore {
                BoardRowSkeleton(rows: 1)
            }
        }
    }

    private func openPendingCabal() {
        guard let cabalID = cabalToOpen else { return }
        cabalToOpen = nil
        open(CabalRoute(id: cabalID))
    }
}

private struct AccountActivityRowView: View {
    let row: AccountActivityRow
    let isLast: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body) private var titleWidthFloor: CGFloat = MonacoRowLayout.baseMinimumTitleWidth

    private var layout: MonacoRowLayout {
        MonacoRowLayout(dynamicTypeSize: dynamicTypeSize, scaledTitleWidthFloor: titleWidthFloor)
    }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 60)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule().padding(.leading, layout.separatorLeadingInset)
                }
            }
            .accessibilityElement(children: .combine)
    }

    @ViewBuilder private var content: some View {
        if layout.isStacked {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    SunkenGlyphMark(systemImage: row.glyph)
                    labels
                }
                amount
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
                SunkenGlyphMark(systemImage: row.glyph)
                labels.frame(minWidth: layout.minimumTitleWidth, alignment: .leading)
                amount.layoutPriority(1)
            }
        }
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(row.title)
                .font(MonacoTheme.Typo.rowTitle)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(layout.titleLineLimit)
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.xs) {
                    date
                    status
                }
                VStack(alignment: .leading, spacing: 2) {
                    date
                    status
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var date: some View {
        Text(row.date)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
    }

    @ViewBuilder private var status: some View {
        if let label = row.status.rowLabel {
            Text(label)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(row.status == .failed ? MonacoTheme.loss : MonacoTheme.warning)
        }
    }

    private var amount: some View {
        Text(row.amount)
            .moneyFont(.row)
            .foregroundStyle(MonacoTheme.ink)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
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
            AccountActivityList(model: model) { path.append(AnyAppRoute($0)) }
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
                .task {
                    let created = await makeModel()
                    model = created
                    await created.load()
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
        case "empty":
            return .preview(empty: true, clock: Date.init)
        default:
            return .preview(empty: false, clock: Date.init)
        }
    }
}
#endif
