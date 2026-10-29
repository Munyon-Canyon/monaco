import MonacoAPI
import MonacoAnalytics
import MonacoCore
import MonacoRanking
import SwiftUI

enum LedgerFeature {
    static let storyShareEnabled = true
}

nonisolated struct LedgerRoute: Identifiable, Hashable, Sendable {
    let id = UUID()
    let week: String
}

struct LedgerSheet: View {
    let week: String
    @State private var model: LedgerModel?
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(week: String, model: LedgerModel? = nil) {
        self.week = week
        _model = State(initialValue: model)
    }

    var body: some View {
        content
            .monacoSheet(
                title: title, titleIdentifier: "ledger-title", detents: .review, showsClose: true,
                closeIdentifier: "ledger-close"
            )
            .accessibilityIdentifier("ledger-sheet")
            .task { await start() }
    }

    private var title: String {
        if case .loaded(let screen) = model?.state { return screen.title }
        return "The Ledger"
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .circle)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(LedgerCopy.loadingLabel)
                .accessibilityIdentifier("ledger-loading")
        case .failed(let error):
            if LedgerCopy.isNotReady(error) {
                notReady
            } else {
                ScreenLoadError(identifier: "ledger-error") { Task { await model?.load() } }
            }
        case .loaded(let screen):
            loaded(screen)
        }
    }

    private var notReady: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            Text(LedgerCopy.notReady)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("ledger-not-ready")
            Button("Close") { dismiss() }
                .buttonStyle(.monacoText)
                .accessibilityIdentifier("ledger-not-ready-close")
        }
        .frame(maxWidth: .infinity)
        .padding(.top, MonacoTheme.Space.l)
    }

    private func loaded(_ screen: LedgerScreen) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            hero(screen)
            stats(screen)
            callsCard(screen)
            if LedgerFeature.storyShareEnabled {
                Button(LedgerCopy.shareTitle) {
                    environment.navigator.open(
                        ShareRoute(.ledger(week: screen.week)), in: .home)
                    dismiss()
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("ledger-share")
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var heroLayout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s))
    }

    private var statsLayout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.s))
    }

    private func hero(_ screen: LedgerScreen) -> some View {
        heroLayout {
            Text(LedgerCopy.hero(bps: screen.returnBps))
                .moneyFigure(.hero)
                .foregroundStyle(heroColor(LedgerCopy.heroTone(bps: screen.returnBps)))
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .lineLimit(1)
                .accessibilityIdentifier("ledger-hero")
            Text(LedgerCopy.heroSubtitle(cabals: screen.cabalCount))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(LedgerCopy.heroSpoken(bps: screen.returnBps, cabals: screen.cabalCount))
    }

    private func heroColor(_ tone: LedgerCopy.Tone) -> Color {
        switch tone {
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .flat: MonacoTheme.primaryText
        }
    }

    private func stats(_ screen: LedgerScreen) -> some View {
        statsLayout {
            stat(LedgerCopy.votedLabel, LedgerCopy.ofTotal(screen.voted, screen.eligible), "voted")
            stat(LedgerCopy.rightLabel, LedgerCopy.ofTotal(screen.right, screen.resolved), "right")
            stat(LedgerCopy.sharpestLabel, LedgerCopy.sharpest(screen.sharpestName), "sharpest")
        }
        .monacoActionCard()
    }

    private func stat(_ label: String, _ value: String, _ id: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(label)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(value)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("ledger-stat-\(id)")
    }

    private func callsCard(_ screen: LedgerScreen) -> some View {
        VStack(spacing: 0) {
            bestCall(screen.bestCall)
            Button {
                openFullLedger()
            } label: {
                MonacoRow(
                    title: LedgerCopy.fullLedgerTitle, subtitle: LedgerCopy.fullLedgerSubtitle, chevron: true,
                    isLast: true
                ) {
                    Image(systemName: "list.bullet")
                        .foregroundStyle(MonacoTheme.secondaryText)
                } trailing: {
                    EmptyView()
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("ledger-full")
        }
        .monacoActionCard(inset: 0)
    }

    @ViewBuilder private func bestCall(_ call: LedgerCall?) -> some View {
        if let call {
            MonacoRow(
                title: LedgerCopy.bestCallTitle(call), subtitle: LedgerCopy.bestCallSubtitle(call)
            ) {
                StockMark(symbol: call.symbol, size: 40, logoURL: call.logoURL.flatMap(URL.init(string:)))
            } trailing: {
                Text(LedgerCopy.signedPercent(bps: call.moveBps))
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(call.moveBps < 0 ? MonacoTheme.loss : MonacoTheme.profit)
            }
            .accessibilityIdentifier("ledger-best-call")
        } else {
            MonacoRow(title: LedgerCopy.noBestCallTitle, subtitle: LedgerCopy.noBestCallSubtitle) {
                Image(systemName: "minus.circle")
                    .foregroundStyle(MonacoTheme.secondaryText)
            } trailing: {
                EmptyView()
            }
            .accessibilityIdentifier("ledger-best-call")
        }
    }

    private func start() async {
        if model == nil { model = LedgerModel(api: environment.api, week: week) }
        AppAnalytics.current.step(.ledger(.opened), properties: ["source": .string("push")])
        if case .idle = model?.state { await model?.load() }
    }

    private func openFullLedger() {
        environment.navigator.open(FullLedgerRoute(week: week), in: .home)
        dismiss()
    }
}
