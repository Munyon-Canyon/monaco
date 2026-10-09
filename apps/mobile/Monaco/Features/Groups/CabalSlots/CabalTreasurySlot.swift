import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalTreasurySlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalTreasuryLive(cabalID: context.cabalID)
    }

    static func solscanURL(for address: String) -> URL? {
        URL(string: "https://solscan.io/account/\(address)")
    }
}

enum CabalTreasurySlotCopy {
    static let header = "Cabal treasury"
    static let warning = "Do not send funds here. Transfers are returned."
    static let solscan = "View on Solscan"
    static let failedThing = "the treasury"
    static let loading = "Loading the treasury"
}

private struct CabalTreasuryLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalModel) private var shared
    @State private var model: CabalModel?

    var body: some View {
        CabalTreasuryView(model: shared ?? model, onReloadFailure: { toasts.show($0) })
            .task(id: cabalID) {
                guard shared == nil else { return }
                let model = self.model ?? CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
                self.model = model
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { visible in
                model?.setVisible(visible)
            }
    }
}

struct CabalTreasuryView: View {
    let model: CabalModel?
    let onReloadFailure: (APIError) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalTreasurySlotCopy.header)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-treasury")
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            onReloadFailure(error)
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 2, markShape: .none)
                .accessibilityElement()
                .accessibilityLabel(CabalTreasurySlotCopy.loading)
                .accessibilityIdentifier("cabal-treasury-loading")
        case .failed:
            MonacoErrorRow(thing: CabalTreasurySlotCopy.failedThing, identifier: "cabal-treasury-failed") {
                Task { await model?.load() }
            }
        case .loaded(let cabal):
            CabalTreasuryCard(address: cabal.treasuryAddress)
        }
    }
}

struct CabalTreasuryCard: View {
    let address: String

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(CabalTreasurySlotCopy.warning)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.warning)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-treasury-warning")
            MonacoGroupedList {
                ReceiptLine(label: "Address", value: .address(address), isLast: solscan == nil)
                    .accessibilityIdentifier("cabal-treasury-address")
                if let solscan {
                    Link(destination: solscan) {
                        MonacoRow(title: CabalTreasurySlotCopy.solscan, chevron: true, isLast: true) {
                            SunkenGlyphMark(systemImage: "safari")
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-treasury-solscan")
                }
            }
        }
    }

    private var solscan: URL? { CabalTreasurySlot.solscanURL(for: address) }
}
