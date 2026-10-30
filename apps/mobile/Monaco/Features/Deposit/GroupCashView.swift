import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupCashView: View {
    enum Phase {
        case loading
        case failed
        case loaded(GroupCashSummary)
    }

    let phase: Phase
    let cabalID: String
    let cabalName: String
    let reload: () async -> Void

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @Environment(\.cabalModel) private var cabalModel
    @State private var showsProposeChooser = false

    private var canPropose: Bool {
        cabalModel.map { $0.cabal?.me?.canVote == true } ?? true
    }

    var body: some View {
        ScrollView {
            content
        }
        .refreshable { await reload() }
        .monacoCanvas()
        .navigationTitle("Cabal cash")
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder private var content: some View {
        switch phase {
        case .loading:
            skeleton
        case .failed:
            MoneyLoadFailure(identifier: "group-cash-error") {
                Task { await reload() }
            }
        case .loaded(let summary):
            loaded(summary)
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
            MonacoRowSkeleton(rows: 2, markShape: .circle)
        }
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading cabal cash")
    }

    private func loaded(_ summary: GroupCashSummary) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            performance(summary)
            MonacoSectionHeader("Idle cash")
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.section)
            MonacoCard(style: .inset) {
                MonacoKeyRows {
                    ReceiptLine(label: "Asset", value: .words(summary.asset))
                    ReceiptLine(label: "Cash in the cabal", value: .data(summary.cash), isLast: !LaunchFeatures.apy)
                    if LaunchFeatures.apy {
                        ReceiptLine(label: "Yield status", value: .words(summary.yieldLabel), isLast: true)
                            .accessibilityIdentifier("group-cash-yield")
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if LaunchFeatures.apy, let notice = summary.notice {
                MonacoNotice(notice.tone == .warn ? .warn : .info, message: notice.message)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.sm)
                    .accessibilityIdentifier("group-cash-notice")
            }
            MonacoSectionHeader("Use cabal cash")
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.section)
            actions
        }
        .padding(.bottom, MonacoTheme.Space.l)
    }

    private func performance(_ summary: GroupCashSummary) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("\(cabalName) · Cash")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)
            Text(summary.cash)
                .moneyFigure(.hero)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityIdentifier("group-cash-amount")
            Text("Unallocated USDC in the cabal pot")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.top, MonacoTheme.Space.sm)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .combine)
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            if canPropose {
                Button {
                    showsProposeChooser = true
                } label: {
                    Text("Propose an investment")
                }
                .buttonStyle(.monacoPrimary)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("group-cash-propose")
                .proposeChooser(isPresented: $showsProposeChooser, cabalID: cabalID)
            }
            Button(action: returnToHoldings) {
                MonacoRow(
                    title: "Return to holdings", subtitle: "View the whole cabal portfolio", chevron: true,
                    isLast: true
                ) {
                    SunkenGlyphMark(icon: .markets)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("group-cash-holdings")
        }
    }

    private func returnToHoldings() {
        let navigator = environment.navigator
        if navigator.path(for: navigator.selectedTab).contains(AnyAppRoute(CabalRoute(id: cabalID))) {
            dismiss()
        } else {
            navigator.open(CabalRoute(id: cabalID), in: navigator.selectedTab)
        }
    }
}

#if DEBUG
final class GroupCashSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-groupCashHarness") else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "ready"
        return AnyView(GroupCashHarnessScreen(mode: mode))
    }
}

private struct GroupCashHarnessScreen: View {
    let mode: String

    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            GroupCashView(phase: phase, cabalID: "sample-big-ballers", cabalName: "Big Ballers", reload: {})
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    private var phase: GroupCashView.Phase {
        let pot = CabalPotSummary(.sampleCashOnly)
        return switch mode {
        case "unavailable": .loaded(GroupCashSummary(pot, yieldStatus: .unavailable))
        case "loading": .loading
        case "error": .failed
        default: .loaded(GroupCashSummary(pot))
        }
    }
}
#endif
