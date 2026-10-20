import MonacoCore
import SwiftUI

struct ReceiptView: View {
    enum Phase {
        case loading
        case loaded(ReceiptContent)
        case failed
    }

    let phase: Phase
    var openAccountActivity: () -> Void = {}
    var openProposal: (String) -> Void = { _ in }
    var reload: () -> Void = {}

    init(
        _ content: ReceiptContent, openAccountActivity: @escaping () -> Void = {},
        openProposal: @escaping (String) -> Void = { _ in }
    ) {
        phase = .loaded(content)
        self.openAccountActivity = openAccountActivity
        self.openProposal = openProposal
    }

    init(phase: Phase, reload: @escaping () -> Void) {
        self.phase = phase
        self.reload = reload
    }

    var body: some View {
        Group {
            switch phase {
            case .loading:
                ReceiptSkeleton()
            case .failed:
                ScrollView {
                    MoneyLoadFailure(identifier: "account-txn-receipt-error", retry: reload)
                }
            case .loaded(let content):
                ScrollView {
                    ReceiptBody(
                        content: content, openAccountActivity: openAccountActivity, openProposal: openProposal)
                }
            }
        }
        .monacoCanvas()
        .navigationTitle("Transaction receipt")
        .navigationBarTitleDisplayMode(.inline)
    }
}

private enum ReceiptMetrics {
    static let heroMark: CGFloat = 54
    static let skeletonBarWidth: CGFloat = 160
    static let disabledOpacity = 0.45
}

private struct ReceiptSkeleton: View {
    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            SkeletonBlock(
                width: ReceiptMetrics.heroMark, height: ReceiptMetrics.heroMark, radius: ReceiptMetrics.heroMark / 2)
            SkeletonBar(width: ReceiptMetrics.skeletonBarWidth)
            SkeletonBar(width: ReceiptMetrics.skeletonBarWidth)
            SkeletonCardBlock()
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading transaction")
    }
}

private struct ReceiptBody: View {
    let content: ReceiptContent
    let openAccountActivity: () -> Void
    let openProposal: (String) -> Void

    var body: some View {
        VStack(spacing: 0) {
            hero
            if let notice = content.notice {
                MonacoNotice(Self.tone(notice.tone), message: notice.message)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.m)
            }
            MonacoCard(style: .inset) {
                MonacoKeyRows { keyRows }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            related
                .padding(.top, MonacoTheme.Space.section)
        }
        .padding(.bottom, MonacoTheme.Space.l)
    }

    private var hero: some View {
        VStack(spacing: 0) {
            mark
                .accessibilityHidden(true)
            Text(content.eyebrow)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.center)
                .padding(.top, MonacoTheme.Space.sm)
            Text(content.amount)
                .moneyFigure(.hero)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityIdentifier("account-txn-receipt-amount")
            StatusBadge(content.badgeText, tone: Self.badgeTone(content.badgeTone))
                .padding(.top, MonacoTheme.Space.m)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.l)
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder private var mark: some View {
        switch content.mark {
        case .glyph(let symbol):
            SunkenGlyphMark(systemImage: symbol, size: ReceiptMetrics.heroMark)
        case .stock(let symbol, let logoURL):
            StockMark(symbol: symbol, size: .receipt, logoURL: logoURL)
        }
    }

    @ViewBuilder private var keyRows: some View {
        let rows = content.keyRows
        ForEach(Array(rows.enumerated()), id: \.element.field) { index, row in
            keyRow(row, isLast: index == rows.count - 1 && content.cabal == nil)
        }
        if let cabal = content.cabal {
            NavigationLink(value: AnyAppRoute(CabalRoute(id: cabal.id))) {
                ReceiptLine(label: "Cabal", value: .words(cabal.name), isLast: true)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("account-txn-receipt-cabal")
        }
    }

    @ViewBuilder
    private func keyRow(_ row: ReceiptContent.KeyRow, isLast: Bool) -> some View {
        switch row.field {
        case .recordID:
            ReceiptRecordRow(label: row.label, value: row.value, isLast: isLast)
        case .when:
            ReceiptLine(label: row.label, value: .data(row.value), isLast: isLast)
                .accessibilityIdentifier("account-txn-receipt-time")
        case .status:
            ReceiptLine(label: row.label, value: .words(row.value), isLast: isLast)
                .accessibilityIdentifier("account-txn-receipt-status")
        case .requestedAmount:
            ReceiptLine(label: row.label, value: .data(row.value), isLast: isLast)
        case .from, .to, .asset:
            ReceiptLine(label: row.label, value: .words(row.value), isLast: isLast)
        }
    }

    @ViewBuilder private var related: some View {
        MonacoGroupedList {
            switch content.related {
            case .accountActivity:
                Button(action: openAccountActivity) {
                    MonacoRow(
                        title: "Account activity", subtitle: "Return to your transfer history", chevron: true
                    ) {
                        SunkenGlyphMark(icon: .receipt)
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("account-txn-receipt-activity")
            case .proposal(let id):
                Button {
                    openProposal(id)
                } label: {
                    MonacoRow(
                        title: "Original proposal", subtitle: "See the decision and discussion", chevron: true
                    ) {
                        SunkenGlyphMark(icon: .receipt)
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("receipt-original-proposal")
            case nil:
                EmptyView()
            }
            explorer
        }
    }

    @ViewBuilder private var explorer: some View {
        if let url = content.explorerURL {
            Link(destination: url) {
                MonacoRow(
                    title: "View on explorer", subtitle: "Opens the confirmed transaction", isLast: true
                ) {
                    SunkenGlyphMark(icon: .propose)
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityHint("Opens Safari")
            .accessibilityIdentifier("account-txn-receipt-solscan")
        } else {
            Button {
            } label: {
                MonacoRow(
                    title: "View on explorer", subtitle: "Requires a confirmed transaction ID", isLast: true
                ) {
                    SunkenGlyphMark(icon: .propose)
                }
            }
            .buttonStyle(.monacoRow)
            .disabled(true)
            .opacity(ReceiptMetrics.disabledOpacity)
            .accessibilityIdentifier("account-txn-receipt-solscan")
        }
    }

    private static func tone(_ tone: ReceiptContent.Tone) -> MonacoNotice.Tone {
        switch tone {
        case .brand: .info
        case .warn: .warn
        case .negative: .error
        }
    }

    private static func badgeTone(_ tone: ReceiptContent.Tone) -> StatusBadge.Tone {
        switch tone {
        case .brand: .positive
        case .warn: .warn
        case .negative: .negative
        }
    }
}

private struct ReceiptRecordRow: View {
    let label: String
    let value: String
    let isLast: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    labelText
                    valueText
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                HStack(spacing: MonacoTheme.Space.sm) {
                    labelText
                        .fixedSize()
                    valueText
                        .frame(maxWidth: .infinity, alignment: .trailing)
                }
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: ReceiptLineMetrics.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast { MonacoRule() }
        }
        .accessibilityElement(children: .combine)
    }

    private var labelText: some View {
        Text(label)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.muted)
    }

    private var valueText: some View {
        Text(value)
            .font(MonacoTheme.Typo.subheadStrong.monospacedDigit())
            .foregroundStyle(MonacoTheme.ink)
            .lineLimit(1)
            .truncationMode(.middle)
            .contextMenu {
                Button("Copy", systemImage: MonacoIcon.copy.symbol) { UIPasteboard.general.string = value }
            }
    }
}
