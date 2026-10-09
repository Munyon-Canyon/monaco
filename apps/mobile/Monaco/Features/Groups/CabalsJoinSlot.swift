import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsJoinSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsSearchSection()
    }
}

struct CabalsSearchSection: View {
    static let debounce: Duration = .milliseconds(250)

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(CabalsSearchFocus.self) private var focus: CabalsSearchFocus?
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: CabalSearchModel?

    init(model: CabalSearchModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let model {
                CabalsSearchContent(model: model, open: open, enter: { enter($0, model) })
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-search")
        .task { await start() }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.isActive ?? false) { _, active in
            focus?.isSearching = active
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

private struct CabalsSearchContent: View {
    @Bindable var model: CabalSearchModel
    let open: (String) -> Void
    let enter: (CabalSearchRow) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSearchField(placeholder: "Find a cabal by name", text: $model.query)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabals-search-field")
            results
        }
        .task(id: model.trimmedQuery) {
            guard model.searched != model.trimmedQuery else { return }
            try? await Task.sleep(for: CabalsSearchSection.debounce)
            guard !Task.isCancelled else { return }
            await model.search()
        }
    }

    @ViewBuilder
    private var results: some View {
        switch model.state {
        case .idle:
            EmptyView()
        case .loading:
            MonacoRowSkeleton(markShape: .tile)
                .accessibilityIdentifier("cabals-search-loading")
        case .empty(let query):
            EmptyState(title: "No cabal called \u{201C}\(query)\u{201D}")
                .accessibilityIdentifier("cabals-search-empty")
        case .failed:
            MonacoErrorRow(thing: "cabals", identifier: "cabals-search-error") { Task { await model.search() } }
        case .rows(let rows):
            MonacoGroupedList {
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    CabalSearchRowView(
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

private struct CabalSearchRowView: View {
    let row: CabalSearchRow
    let isEntering: Bool
    let isLast: Bool
    let open: () -> Void
    let enter: () -> Void

    var body: some View {
        Button(action: open) {
            MonacoRow(title: row.name, subtitle: row.detail, isLast: isLast, trailingIsInteractive: true) {
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
            Text("Request sent")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("cabals-search-requested")
        case .member:
            Text("Joined")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("cabals-search-joined")
        }
    }

    private func rowButton(_ title: String, label: String) -> some View {
        Button(title, action: enter)
            .buttonStyle(.monacoCompact)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
            .disabled(isEntering)
            .accessibilityLabel(label)
            .accessibilityIdentifier("cabals-search-enter-\(row.id)")
    }
}

#if DEBUG
final class CabalsSearchSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCabalsSearchSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains(launchArgument) else { return nil }
        return AnyView(
            NavigationStack {
                ScrollView {
                    CabalsSearchSection(model: CabalSearchModel.preview())
                }
                .monacoCanvas()
                .navigationTitle(CabalsTab.title)
            }
        )
    }
}
#endif
