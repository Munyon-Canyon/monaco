import MonacoAPI
import MonacoAnalytics
import MonacoCore
import MonacoRanking
import SwiftUI

enum CallBoardMetrics {
    static let skeletonRows = 6
}

struct LeaderboardView: View {
    let tab: CallBoardTab
    let period: CallPeriod
    var preview: CallBoardModel?

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CallBoardModel?
    @State private var viewerRowVisible = true
    @State private var showsInfo = false

    var body: some View {
        Group {
            if let model {
                page(model)
            } else {
                Color.clear
            }
        }
        .monacoCanvas()
        .navigationTitle(CallBoardCopy.title)
        .navigationBarTitleDisplayMode(.large)
        .navigationBarBackButtonHidden()
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                MonacoGlassButton(
                    systemImage: "chevron.left", accessibilityLabel: CallBoardCopy.back, style: .quiet,
                    placement: .toolbar
                ) { dismiss() }
            }
            ToolbarItem(placement: .topBarTrailing) {
                MonacoGlassButton(
                    systemImage: "info.circle", accessibilityLabel: CallBoardCopy.infoLabel, style: .quiet,
                    placement: .toolbar
                ) {
                    AppAnalytics.current.step(.leaderboard(.infoOpened))
                    showsInfo = true
                }
                .accessibilityIdentifier("leaderboard-info")
            }
        }
        .sheet(isPresented: $showsInfo) { CallBoardInfoSheet() }
        .analyticsScreen("leaderboard")
        .accessibilityIdentifier("leaderboard-page")
        .onAppear { model = model ?? preview ?? makeModel() }
        .task(id: model == nil) {
            guard let model, preview == nil else { return }
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func makeModel() -> CallBoardModel {
        CallBoardModel(
            api: environment.api, hints: environment.hints, board: tab.board, period: period, filter: tab.filter)
    }

    private func page(_ model: CallBoardModel) -> some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 0, pinnedViews: [.sectionHeaders]) {
                CallBoardHeader(
                    title: headerTitle(model), range: headerRange(model), selection: periodBinding(model))
                Section {
                    content(model)
                } header: {
                    MonacoUnderlineTabs(
                        CallBoardTab.allCases, selection: tabBinding(model),
                        identifier: { "leaderboard-tab-\($0.rawValue)" },
                        label: CallBoardCopy.tabLabel
                    )
                    .monacoUnderlineTabsPinned()
                }
            }
        }
        .refreshable { await model.load() }
        .safeAreaInset(edge: .bottom) {
            if case .loaded(let screen) = model.state, screen.showsDocked(viewerRowVisible: viewerRowVisible),
                let card = screen.card
            {
                CallBoardDocked(card: card) { share(card) }
            }
        }
    }

    private func share(_ card: CallBoardCard) {
        guard let standing = card.standing else { return }
        Task { @MainActor in
            do {
                let code = try await environment.api.read { try await $0.getMyReferralCode().ok.body.json }
                guard let links = InviteLinks(code) else { return }
                SystemShare.present(CallBoardSharePayload.make(standing: standing, inviteURL: links.shareURL))
            } catch {
                toasts.show(APIError(error))
            }
        }
    }

    @ViewBuilder private func content(_ model: CallBoardModel) -> some View {
        switch model.state {
        case .idle, .loading:
            BoardRowSkeleton(rows: CallBoardMetrics.skeletonRows)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(CallBoardCopy.loadingLabel)
        case .failed:
            EmptyState(
                title: CallBoardCopy.errorTitle, message: CallBoardCopy.errorMessage,
                actionTitle: CallBoardCopy.tryAgain, actionIdentifier: "leaderboard-error",
                systemImage: "wifi.exclamationmark"
            ) { Task { await model.load() } }
        case .loaded(let screen):
            if screen.isEmpty {
                EmptyState(
                    title: CallBoardCopy.emptyTitle, message: screen.emptyMessage,
                    systemImage: "chart.line.uptrend.xyaxis"
                )
                .accessibilityIdentifier("leaderboard-empty")
            } else {
                CallBoardBody(
                    screen: screen, viewerRowVisible: $viewerRowVisible,
                    onLastAppear: { Task { await model.loadMore() } })
            }
        }
    }

    private func headerTitle(_ model: CallBoardModel) -> String {
        if case .loaded(let screen) = model.state { return screen.periodTitle }
        return CallBoardCopy.periodTitle(model.period)
    }

    private func headerRange(_ model: CallBoardModel) -> String? {
        if case .loaded(let screen) = model.state { return screen.periodRange }
        return nil
    }

    private func periodBinding(_ model: CallBoardModel) -> Binding<CallPeriod> {
        Binding(
            get: { model.period },
            set: { period in
                AppAnalytics.current.step(
                    .leaderboard(.periodSelected), properties: ["period": .string(period.rawValue)])
                Task { await model.select(period: period) }
            })
    }

    private func tabBinding(_ model: CallBoardModel) -> Binding<CallBoardTab> {
        Binding(
            get: { model.tab },
            set: { tab in
                AppAnalytics.current.step(.leaderboard(.tabSelected), properties: ["tab": .string(tab.rawValue)])
                Task { await model.select(tab: tab) }
            })
    }
}

struct CallBoardBody: View {
    let screen: CallBoardScreen
    @Binding var viewerRowVisible: Bool
    private let onLastAppear: () -> Void
    @State private var showMoment: Bool

    private let seen = CallBoardSeenStore()

    init(screen: CallBoardScreen, viewerRowVisible: Binding<Bool>, onLastAppear: @escaping () -> Void = {}) {
        self.screen = screen
        _viewerRowVisible = viewerRowVisible
        self.onLastAppear = onLastAppear
        _showMoment = State(initialValue: screen.moment.map(CallBoardSeenStore().shouldShow) ?? false)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if showMoment, let moment = screen.moment {
                CallBoardMomentCard(moment: moment)
                    .task(id: moment.id) {
                        try? await Task.sleep(for: .seconds(1))
                        seen.markSeen(moment)
                    }
            } else if let card = screen.card {
                CallBoardYouCard(card: card)
            }
            if let unranked = screen.unrankedCard {
                CallBoardUnrankedCardView(card: unranked)
            }
            if !screen.top.isEmpty {
                CallBoardSectionTitle(text: screen.podiumTitle)
                CallBoardPodium(rows: screen.top)
            }
            if !screen.around.isEmpty {
                CallBoardSectionTitle(text: screen.aroundTitle, identifier: "leaderboard-section-ranked")
                around
            }
            if !screen.unrankedRows.isEmpty {
                CallBoardSectionTitle(
                    text: CallBoardCopy.unrankedTitle, count: screen.unrankedRows.count,
                    identifier: "leaderboard-section-unranked")
                CallBoardUnrankedRows(rows: screen.unrankedRows)
            }
        }
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder private var around: some View {
        if let mine = screen.card?.me, let index = screen.around.firstIndex(where: { $0.id == mine.id }) {
            CallBoardRows(rows: Array(screen.around[..<index]), bandID: screen.bandID)
            CallBoardRows(
                rows: [screen.around[index]], bandID: screen.bandID,
                onLastAppear: index == screen.around.count - 1 ? onLastAppear : {}
            )
            .onScrollVisibilityChange { viewerRowVisible = $0 }
            CallBoardRows(rows: Array(screen.around[(index + 1)...]), bandID: screen.bandID, onLastAppear: onLastAppear)
        } else {
            CallBoardRows(rows: screen.around, bandID: screen.bandID, onLastAppear: onLastAppear)
        }
    }
}

struct CallBoardSectionTitle: View {
    let text: String
    var count: Int?
    var identifier: String?

    var body: some View {
        (Text(text).foregroundStyle(MonacoTheme.primaryText)
            + Text(count.map { " \($0)" } ?? "").foregroundStyle(MonacoTheme.secondaryText))
            .font(MonacoTheme.Typo.section)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.l)
            .padding(.bottom, MonacoTheme.Space.s)
            .accessibilityAddTraits(.isHeader)
            .accessibilityIdentifier(identifier ?? "leaderboard-section")
    }
}
