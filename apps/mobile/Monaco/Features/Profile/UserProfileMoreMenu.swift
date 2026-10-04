import SwiftUI

struct UserProfileMoreMenu: View {
    var body: some View {
        Menu {
            Section("Report and block open soon.") {
                Button("Report") {}
                    .disabled(true)
                Button("Block") {}
                    .disabled(true)
            }
        } label: {
            Image(systemName: "ellipsis")
                .frame(minWidth: 44, minHeight: 44)
        }
        .accessibilityLabel("More")
        .accessibilityIdentifier("user-profile-more")
    }
}
