import MonacoAPI
import MonacoCore
import SwiftUI

struct PublicCabalReturn: Equatable {
    let percentText: String
    let pnlText: String?
}

struct PublicCabalView: View {
    let cabal: CabalModel
    var isPrivate = false
    var returns: PublicCabalReturn?
    let retry: () -> Void

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        content
            .navigationTitle(cabal.cabal?.name ?? "")
            .navigationBarTitleDisplayMode(.inline)
            .monacoCanvas()
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("public-cabal")
    }

    @ViewBuilder private var content: some View {
        if isPrivate {
            EmptyState(
                title: "This cabal is private", message: "Private holdings and conversations are not visible.",
                actionTitle: "Back", actionIdentifier: "public-cabal-back", systemImage: MonacoIcon.lock.symbol
            ) { dismiss() }
            .frame(maxHeight: .infinity)
            .accessibilityIdentifier("public-cabal-private")
        } else {
            switch cabal.state {
            case .idle, .loading:
                PublicCabalSkeleton()
            case .failed:
                CabalFullScreenError(identifier: "public-cabal-failed", retry: retry)
            case .loaded(let loaded):
                loadedContent(loaded)
            }
        }
    }

    private func loadedContent(_ loaded: Components.Schemas.Cabal) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                CabalHeroCover(cabal: loaded, tall: true)
                returnsBlock
                membership(loaded)
                cta(loaded)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .refreshable { retry() }
    }

    @ViewBuilder private var returnsBlock: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            if let returns {
                PercentText(percentReturn: returns.percentText, style: .large)
                    .accessibilityIdentifier("public-cabal-return")
                Text([returns.pnlText, "1M P&L"].compactMap { $0 }.joined(separator: " \u{00B7} "))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            } else {
                Text("\u{2014}")
                    .moneyFont(.large)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityLabel("Return unavailable")
                    .accessibilityIdentifier("public-cabal-return")
                Text(CabalCopy.returnsOpenSoon)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private func membership(_ loaded: Components.Schemas.Cabal) -> some View {
        let summary = CabalRulesSummary(loaded)
        return VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Membership")
            MonacoCard(style: .inset) {
                MonacoKeyRows {
                    ReceiptLine(label: "Joining", value: .words(summary.join.value))
                        .accessibilityIdentifier("public-cabal-joining")
                    ReceiptLine(
                        label: "Voting",
                        value: .words(
                            loaded.rules.voterMode == CabalVoterMode.picked.rawValue ? "Eligible members" : "Everyone")
                    )
                    .accessibilityIdentifier("public-cabal-voting")
                    ReceiptLine(label: "Voting window", value: .words(summary.expiry.value), isLast: true)
                        .accessibilityIdentifier("public-cabal-window")
                }
            }
        }
    }

    @ViewBuilder private func cta(_ loaded: Components.Schemas.Cabal) -> some View {
        if loaded.me != nil {
            Button("Open your cabal", action: retry)
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("public-cabal-open")
        } else {
            CabalJoinSection(cabalID: loaded.id)
                .padding(.horizontal, -MonacoTheme.Space.gutter)
        }
    }
}

private struct PublicCabalSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonBlock(height: CabalHeroMetrics.tallSkeletonHeight, radius: MonacoTheme.Radius.identity)
            SkeletonBar()
            SkeletonBar()
            SkeletonCardBlock()
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityElement()
        .accessibilityLabel("Loading this cabal")
        .accessibilityIdentifier("public-cabal-loading")
    }
}
