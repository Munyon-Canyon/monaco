import MonacoAPI
import MonacoCore
import SwiftUI

struct DiscoverCabalsView: View {
    static let debounce: Duration = .milliseconds(250)

    let query: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: CabalSearchModel?

    init(query: String = "", model: CabalSearchModel? = nil) {
        self.query = query
        _model = State(initialValue: model)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text("Find a cabal")
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityAddTraits(.isHeader)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.m)
                if let model {
                    DiscoverCabalsContent(model: model, open: open, enter: { enter($0, model) })
                }
            }
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollDismissesKeyboard(.interactively)
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle("Discover cabals")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-search")
        .task { await start() }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
            if toast.message == CabalEntry.requestedToast {
                Task { await environment.pushPrePrompt.noteCabalJoined(after: toasts) }
            }
        }
    }

    private func start() async {
        let model = preparedModel()
        if model.query.isEmpty { model.query = query }
        refresh?.register("cabals-search") { await model.refresh() }
        await model.observe()
    }

    private func preparedModel() -> CabalSearchModel {
        if let model { return model }
        let created = CabalSearchModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }

    private func open(_ cabalID: String) {
        environment.navigator.open(CabalRoute(id: cabalID), in: .cabals)
    }

    private func enter(_ row: CabalSearchRow, _ model: CabalSearchModel) {
        Task {
            guard let cabalID = await model.enter(row) else { return }
            Haptics.selection()
            open(cabalID)
        }
    }
}

private struct DiscoverCabalsContent: View {
    @Bindable var model: CabalSearchModel
    let open: (String) -> Void
    let enter: (CabalSearchRow) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSearchField(placeholder: "Search cabals", text: $model.query)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.m)
                .accessibilityIdentifier("cabals-search-field")
            results
        }
        .task(id: model.trimmedQuery) {
            guard model.searched != model.trimmedQuery else { return }
            try? await Task.sleep(for: DiscoverCabalsView.debounce)
            guard !Task.isCancelled else { return }
            await model.search()
        }
    }

    @ViewBuilder
    private var results: some View {
        switch model.state {
        case .idle:
            DiscoverBrowse(open: open)
        case .loading:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.card)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoRowSkeleton(rows: 4, markShape: .tile)
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Searching cabals")
            .accessibilityIdentifier("cabals-search-loading")
        case .empty:
            EmptyState(
                title: "No matching cabals", message: "Try another name.", systemImage: MonacoIcon.search.symbol
            )
            .accessibilityIdentifier("cabals-search-empty")
        case .failed:
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "cabals-search-error",
                systemImage: MonacoIcon.offline.symbol, action: { Task { await model.search() } }
            )
        case .rows(let rows):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if let pending = rows.first(where: { $0.action == .requested }) {
                    MonacoNotice(.warn, message: "Your request to join \(pending.name) is pending.")
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .accessibilityIdentifier("cabals-search-pending-notice")
                }
                MonacoGroupedList {
                    ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                        DiscoverCabalRow(
                            row: row,
                            isEntering: model.entering.contains(row.id),
                            isLast: index == rows.count - 1,
                            open: { open(row.id) },
                            enter: { enter(row) }
                        )
                        .onAppear {
                            guard index == rows.count - 1, model.canLoadMore else { return }
                            Task { await model.loadMore() }
                        }
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("cabals-search-results")
            }
        }
    }
}

private struct DiscoverBrowse: View {
    let open: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            LeaderboardHost(board: .cabals, refreshKey: "discover-board") { loader in
                LeaderboardBoardList(
                    loader: loader, skeletonRows: 4, failureThing: "cabals", identifier: "cabals-browse",
                    showsEmpty: false, showsPaging: false,
                    rowContent: { row, isLast in
                        Button {
                            open(row.id)
                        } label: {
                            MonacoRow(title: row.name, chevron: true, isLast: isLast) {
                                CabalMark(groupId: row.id, name: row.name, pictureUrl: row.pictureURL)
                            }
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("cabals-browse-row-\(row.id)")
                    })
            }
            Text("Browsing every public cabal opens soon. Search by name to find more.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabals-browse-coming")
        }
    }
}

private struct DiscoverCabalRow: View {
    let row: CabalSearchRow
    let isEntering: Bool
    let isLast: Bool
    let open: () -> Void
    let enter: () -> Void

    @Environment(\.accountRestricted) private var accountRestricted

    var body: some View {
        Button(action: open) {
            MonacoRow(
                title: row.name, subtitle: row.detail, chevron: true, isLast: isLast, trailingIsInteractive: true
            ) {
                CabalMark(groupId: row.id, name: row.name, pictureUrl: row.pictureURL)
            } trailing: {
                trailing
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabals-search-result-\(row.id)")
    }

    @ViewBuilder
    private var trailing: some View {
        switch row.action {
        case .request:
            switch row.joinPolicy {
            case .open: rowButton("Join", label: "Join \(row.name)")
            case .request: rowButton("Request", label: "Request to join \(row.name)")
            }
        case .requested:
            StatusBadge("Request pending", tone: .warn)
                .accessibilityIdentifier("cabals-search-requested")
        case .member:
            StatusBadge("Joined", tone: .neutral)
                .accessibilityIdentifier("cabals-search-joined")
        }
    }

    private func rowButton(_ title: String, label: String) -> some View {
        Button(title, action: enter)
            .buttonStyle(.monacoCompact)
            .disabled(isEntering || accountRestricted)
            .accessibilityLabel(label)
            .accessibilityIdentifier("cabals-search-enter-\(row.id)")
    }
}

#if DEBUG
final class DiscoverCabalsSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoDiscoverSample"

    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        let state = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "results"
        switch state {
        case "loading":
            SampleAPIProtocol.install(SampleAPIScript(mode: .hang))
            return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [DiscoverCabalsRoute(query: "week")]))
        case "error":
            SampleAPIProtocol.install(SampleAPIScript())
            return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [DiscoverCabalsRoute(query: "week")]))
        case "browse":
            SampleAPIProtocol.install(SampleAPIScript())
            return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [DiscoverCabalsRoute()]))
        default:
            return AnyView(DiscoverSampleScreen(state: state))
        }
    }
}

private struct DiscoverSampleScreen: View {
    let state: String

    var body: some View {
        NavigationStack {
            DiscoverCabalsView(query: state == "noResults" ? "zzzz" : "week", model: model)
        }
    }

    private var model: CabalSearchModel {
        let items = Components.Schemas.CabalSearchItem.samples
        switch state {
        case "noResults":
            return CabalSearchModel.preview([])
        case "requestPending":
            return CabalSearchModel.preview(items)
        default:
            return CabalSearchModel.preview(items.filter { $0.myAccessRequestStatus == nil })
        }
    }
}
#endif
