import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalActivitySlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalActivityLive(cabalID: context.cabalID)
    }
}

enum CabalActivityCopy {
    static let header = "Activity"
    static let seeAll = "See all"
    static let emptyTitle = "Nothing yet"
    static let emptyMessage = "Money added and trades show up here."
    static let failedThing = "activity"
    static let loading = "Loading activity"
    static let retrySwap = "Retry"
}

extension View {
    func cabalActivityToasts(_ model: CabalActivityModel?, in toasts: ToastCenter) -> some View {
        onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
        }
    }
}

private struct CabalActivityLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: CabalActivityModel?

    var body: some View {
        CabalActivitySection(model: model)
            .task {
                let model = preparedModel()
                refresh?.register("cabal-activity") { await model.refresh() }
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .cabalActivityToasts(model, in: toasts)
    }

    private func preparedModel() -> CabalActivityModel {
        if let model { return model }
        let created = CabalActivityModel(
            cabalID: cabalID, api: environment.api, hints: environment.hints, clock: Date.init)
        model = created
        return created
    }
}

struct CabalActivitySection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    let model: CabalActivityModel?

    var body: some View {
        if model?.phase != .hidden {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                header
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                CabalActivityContent(
                    opensThroughNavigator: true, model: model, rows: model?.firstFive ?? [], skeletonRows: 3)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-activity")
        } else {
            Color.clear.frame(height: 0)
        }
    }

    @ViewBuilder private var header: some View {
        if let model, model.hasMore {
            MonacoSectionHeader(
                CabalActivityCopy.header, trailing: CabalActivityCopy.seeAll,
                actionIdentifier: "cabal-activity-see-all"
            ) {
                environment.navigator.open(
                    CabalActivityListRoute(cabalID: model.cabalID),
                    in: hostMainTab ?? environment.navigator.selectedTab)
            }
        } else {
            MonacoSectionHeader(CabalActivityCopy.header)
        }
    }
}

struct CabalActivityContent: View {
    var opensThroughNavigator = false
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    let model: CabalActivityModel?
    let rows: [ActivityRow]
    let skeletonRows: Int
    var onLastRowAppear: (() -> Void)?

    var body: some View {
        switch model?.phase ?? .loading {
        case .loading:
            MonacoRowSkeleton(rows: skeletonRows, markShape: .tile)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(CabalActivityCopy.loading)
                .accessibilityIdentifier("cabal-activity-loading")
        case .empty:
            EmptyState(title: CabalActivityCopy.emptyTitle, message: CabalActivityCopy.emptyMessage)
                .accessibilityIdentifier("cabal-activity-empty")
        case .failed:
            MonacoErrorRow(thing: CabalActivityCopy.failedThing, identifier: "cabal-activity-error") {
                Task { await model?.load() }
            }
        case .loaded:
            list
        case .hidden:
            EmptyView()
        }
    }

    @ViewBuilder private func receiptLink(_ row: ActivityRow, @ViewBuilder label: () -> some View) -> some View {
        let cabalID = model?.cabalID ?? ""
        if opensThroughNavigator, let hostMainTab {
            Button {
                environment.navigator.openTransaction(
                    cabalID: cabalID, transactionID: row.id, in: hostMainTab)
            } label: {
                label()
            }
        } else {
            NavigationLink(value: AnyAppRoute(TransactionRoute(cabalID: cabalID, transactionID: row.id))) {
                label()
            }
        }
    }

    private var list: some View {
        let loadingMore = model?.isLoadingMore == true
        return MonacoGroupedList(rules: loadingMore ? .top : .both) {
            LazyVStack(spacing: 0) {
                ForEach(rows) { row in
                    receiptLink(row) {
                        CabalActivityRowView(row: row, isLast: row.id == rows.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-activity-row-\(row.id)")
                    .onAppear {
                        guard row.id == rows.last?.id else { return }
                        onLastRowAppear?()
                    }
                }
            }
            if loadingMore {
                MonacoRowSkeleton(rows: 1, markShape: .tile)
            }
        }
    }
}

private struct CabalActivityRowView: View {
    let row: ActivityRow
    let isLast: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule().padding(
                        .leading, MonacoRowLayout(dynamicTypeSize: dynamicTypeSize).separatorLeadingInset)
                }
            }
            .accessibilityElement(children: .combine)
    }

    @ViewBuilder private var content: some View {
        if dynamicTypeSize.isAccessibilitySize {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    mark
                    titleText
                    subtitle.font(MonacoTheme.Typo.caption)
                    amountText
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                chevron
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
                mark
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    titleText
                    subtitle.font(MonacoTheme.Typo.caption)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                amountText
                    .layoutPriority(1)
                chevron
            }
        }
    }

    @ViewBuilder private var mark: some View {
        if row.kind.isSwap, let symbol = row.symbol {
            StockMark(symbol: symbol)
        } else {
            SunkenGlyphMark(systemImage: row.glyph)
        }
    }

    private var titleText: some View {
        Text(row.title)
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.ink)
            .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 2)
    }

    @ViewBuilder private var amountText: some View {
        if let amount = row.amount {
            Text(amount)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
        }
    }

    private var chevron: some View {
        Image(systemName: "chevron.right")
            .font(.footnote.weight(.semibold))
            .foregroundStyle(MonacoTheme.tertiaryText)
            .accessibilityHidden(true)
    }

    private var subtitle: Text {
        let age = Text(row.age).foregroundStyle(MonacoTheme.muted)
        guard let label = row.status.rowLabel else { return age }
        let color = row.status == .failed ? MonacoTheme.loss : MonacoTheme.warning
        return Text("\(age)\(Text(" · \(label)").foregroundStyle(color))")
    }
}
