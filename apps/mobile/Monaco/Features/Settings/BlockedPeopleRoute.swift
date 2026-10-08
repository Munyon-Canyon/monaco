import SwiftUI

nonisolated struct BlockedPeopleRoute: AppRoute {
    @MainActor func destination() -> some View {
        BlockedPeopleView()
    }
}

struct BlockedPeopleView: View {
    var body: some View {
        ScrollView {
            Text("People you block show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityIdentifier("blocked-people-coming")
        }
        .monacoCanvas()
        .navigationTitle("Blocked people")
        .navigationBarTitleDisplayMode(.inline)
    }
}
