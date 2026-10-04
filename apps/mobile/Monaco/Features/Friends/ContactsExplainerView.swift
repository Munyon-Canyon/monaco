import SwiftUI

struct ContactsExplainerView: View {
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ScrollView {
            Text(
                "See which of your contacts are already on Monaco. Only scrambled numbers leave your phone, never your address book."
            )
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.ink)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(MonacoTheme.Space.m)
        }
        .safeAreaInset(edge: .bottom) {
            VStack(spacing: MonacoTheme.Space.s) {
                Text("Finding friends opens soon.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                Button("Find friends") {}
                    .buttonStyle(.monacoPrimary)
                    .disabled(true)
                    .accessibilityIdentifier("friends-find-coming")
                Button("Not now") { dismiss() }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("friends-not-now")
            }
            .monacoFullWidthButtons()
            .padding(MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Friends on Monaco")
        .navigationBarTitleDisplayMode(.inline)
    }
}
