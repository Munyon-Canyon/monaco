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

struct HomeSharedErrorRow: View {
    @Environment(\.homeReads) private var reads
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?

    var body: some View {
        if reads?.plan.showsSharedRow == true {
            MonacoErrorRow(thing: "Home", identifier: "home-offline-error") {
                Task { await refresh?.run() }
            }
            .padding(.top, MonacoTheme.Space.m)
        }
    }
}
