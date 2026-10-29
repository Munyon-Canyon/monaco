import MonacoAPI
import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var homeReads: HomeReads?
}

extension Optional where Wrapped == HomeReads {
    func showsOwnRow(_ slot: HomeReadSlot) -> Bool {
        self?.plan.showsOwnRow(slot) ?? true
    }
}

struct HomeSharedErrorState: View {
    @Environment(\.homeReads) private var reads
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?

    var body: some View {
        if reads?.plan.showsSharedRow == true {
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "home-offline-error", systemImage: MonacoIcon.offline.symbol
            ) {
                Task { await refresh?.run() }
            }
        }
    }
}

struct HomeGate {
    let summary: PortfolioSummary?
    let sharedFailure: Bool
    let cashMicros: Int64

    init(portfolio: PortfolioModel?, reads: HomeReads?, balance: LoadState<AccountBalance>) {
        summary = portfolio?.summary
        sharedFailure = reads?.plan.showsSharedRow == true
        if case .loaded(let loaded) = balance { cashMicros = loaded.availableMicros } else { cashMicros = 0 }
    }

    var variant: HomeVariant? {
        summary.map { HomeVariant(summary: $0, cashMicros: cashMicros) }
    }

    var isLoading: Bool { summary == nil && !sharedFailure }

    var firstUse: Bool { !sharedFailure && HomeFirstUse.applies(to: summary) }

    func shows(_ block: HomeBlock) -> Bool {
        !sharedFailure && HomeFirstUse.shows(block, summary: summary)
    }

    func shows(_ rule: (HomeVariant) -> Bool) -> Bool {
        guard !sharedFailure, let variant else { return false }
        return rule(variant)
    }
}

extension View {
    func homeSlotHidden(_ hidden: Bool) -> some View {
        frame(maxHeight: hidden ? 0 : nil)
            .clipped()
            .opacity(hidden ? 0 : 1)
            .allowsHitTesting(!hidden)
            .accessibilityHidden(hidden)
    }
}
