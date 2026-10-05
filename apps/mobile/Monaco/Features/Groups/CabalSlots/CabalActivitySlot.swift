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
    static let failed = "Couldn't load activity."
    static let retry = "Try again"
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
    let model: CabalActivityModel?

    var body: some View {
        if model?.phase != .hidden {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                header
                    .padding(.horizontal, MonacoTheme.Space.m)
                CabalActivityContent(model: model, rows: model?.firstFive ?? [], skeletonRows: 3)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-activity")
        }
    }

    @ViewBuilder private var header: some View {
        if let model, model.hasMore {
            NavigationLink(value: AnyAppRoute(CabalActivityListRoute(cabalID: model.cabalID))) {
                MonacoSectionHeader(CabalActivityCopy.header, trailing: CabalActivityCopy.seeAll)
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("cabal-activity-see-all")
        } else {
            MonacoSectionHeader(CabalActivityCopy.header)
        }
    }
}

struct CabalActivityContent: View {
    let model: CabalActivityModel?
    let rows: [ActivityRow]
    let skeletonRows: Int
    var onLastRowAppear: (() -> Void)?

    var body: some View {
        switch model?.phase ?? .loading {
        case .loading:
            BoardRowSkeleton(rows: skeletonRows)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(CabalActivityCopy.loading)
                .accessibilityIdentifier("cabal-activity-loading")
        case .empty:
            EmptyState(title: CabalActivityCopy.emptyTitle, message: CabalActivityCopy.emptyMessage)
                .accessibilityIdentifier("cabal-activity-empty")
        case .failed:
            EmptyState(title: CabalActivityCopy.failed, actionTitle: CabalActivityCopy.retry) {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("cabal-activity-error")
        case .loaded:
            list
        case .hidden:
            EmptyView()
        }
    }

    private var list: some View {
        let loadingMore = model?.isLoadingMore == true
        return MonacoGroupedList(rules: loadingMore ? .top : .both) {
            LazyVStack(spacing: 0) {
                ForEach(rows) { row in
                    NavigationLink(
                        value: AnyAppRoute(
                            TransactionRoute(cabalID: model?.cabalID ?? "", transactionID: row.id))
                    ) {
                        CabalActivityRowView(row: row, isLast: row.id == rows.last?.id) {
                            Task { await model?.retrySwap(row) }
                        }
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
                BoardRowSkeleton(rows: 1)
            }
        }
    }
}

private struct CabalActivityRowView: View {
    let row: ActivityRow
    let isLast: Bool
    let retry: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            SunkenGlyphMark(systemImage: row.glyph)
            VStack(alignment: .leading, spacing: 2) {
                Text(row.title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(2)
                HStack(spacing: 0) {
                    subtitle
                    if row.offersRetry {
                        Text(" · ").foregroundStyle(MonacoTheme.loss)
                        Button(CabalActivityCopy.retrySwap, action: retry)
                            .buttonStyle(.plain)
                            .foregroundStyle(MonacoTheme.loss)
                            .underline()
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                            .accessibilityIdentifier("cabal-activity-retry-\(row.id)")
                    }
                }
                .font(MonacoTheme.Typo.caption)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if let amount = row.amount {
                Text(amount)
                    .moneyFont(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                    .layoutPriority(1)
            }
            Image(systemName: "chevron.right")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(MonacoTheme.tertiaryText)
                .accessibilityHidden(true)
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: 60)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, MonacoTheme.Space.m)
            }
        }
        .accessibilityElement(children: row.offersRetry ? .contain : .combine)
    }

    private var subtitle: Text {
        let age = Text(row.age).foregroundStyle(MonacoTheme.muted)
        guard let label = row.status.rowLabel else { return age }
        let color = row.status == .failed ? MonacoTheme.loss : MonacoTheme.warning
        return Text("\(age)\(Text(" · \(label)").foregroundStyle(color))")
    }
}
