import MonacoAPI
import MonacoAnalytics
import MonacoRanking
import SwiftUI

struct FullLedgerView: View {
    let week: String
    @State private var model: LedgerModel?
    @Environment(AppEnvironment.self) private var environment

    init(week: String, model: LedgerModel? = nil) {
        self.week = week
        _model = State(initialValue: model)
    }

    var body: some View {
        content
            .monacoCanvas()
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .analyticsScreen("full_ledger")
            .task { await start() }
    }

    private var title: String {
        if case .loaded(let screen) = model?.state { return screen.title }
        return LedgerCopy.fullLedgerTitle
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 5, markShape: .circle)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(LedgerCopy.loadingLabel)
                .accessibilityIdentifier("full-ledger-loading")
        case .failed(let error):
            if LedgerCopy.isNotReady(error) {
                Text(LedgerCopy.notReady)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .padding(.top, MonacoTheme.Space.l)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .accessibilityIdentifier("full-ledger-not-ready")
            } else {
                ScreenLoadError(identifier: "full-ledger-error") { Task { await model?.load() } }
            }
        case .loaded(let screen):
            ScrollView { sections(screen) }
                .accessibilityIdentifier("full-ledger-page")
        }
    }

    private func sections(_ screen: LedgerScreen) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoSectionHeader(LedgerCopy.everyCall)
            if screen.calls.isEmpty {
                Text(LedgerCopy.noCalls)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            ForEach(screen.calls) { call in
                callRow(call)
            }
            if let miss = screen.miss {
                MonacoSectionHeader(LedgerCopy.theMiss)
                callRow(miss)
            }
            if let reply = screen.topReply {
                MonacoSectionHeader(LedgerCopy.topReply)
                replyCard(reply)
            }
        }
        .padding(.bottom, MonacoTheme.Space.xl)
    }

    private func callRow(_ call: LedgerCall) -> some View {
        Button {
            environment.navigator.open(ProposalRoute(proposalID: call.proposalID), in: .home)
        } label: {
            MonacoRow(title: LedgerCopy.callRowTitle(call), subtitle: LedgerCopy.callRowSubtitle(call)) {
                StockMark(symbol: call.symbol, size: 40, logoURL: call.logoURL.flatMap(URL.init(string:)))
            } trailing: {
                Text(LedgerCopy.signedPercent(bps: call.moveBps))
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(call.moveBps < 0 ? MonacoTheme.loss : MonacoTheme.profit)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("full-ledger-call-\(call.proposalID)")
    }

    private func replyCard(_ reply: LedgerReply) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(photoURL: reply.authorPhotoURL, displayName: reply.authorName, size: 32)
                Text(reply.authorName)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
            }
            Text(reply.excerpt)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(LedgerCopy.replyCaption(reply))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .monacoActionCard()
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("full-ledger-top-reply")
    }

    private func start() async {
        if model == nil { model = LedgerModel(api: environment.api, week: week) }
        AppAnalytics.current.step(.fullLedger(.opened))
        if case .idle = model?.state { await model?.load() }
    }
}
