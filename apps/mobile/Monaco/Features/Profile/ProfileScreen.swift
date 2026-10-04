import SwiftUI

struct ProfileScreen: View {
    static let sections: [any ProfileSection.Type] = [
        ProfileHeaderSlot.self,
        ProfileFollowCountsSlot.self,
        ProfileStatsSlot.self,
        ProfileBalanceSlot.self,
        ProfileCabalsSlot.self,
        ProfileInviteSlot.self,
        ProfileFindFriendsSlot.self,
        ProfileSettingsSlot.self,
    ]

    static let signOutTitle = "Sign out of Monaco?"
    static let signOutMessage = "Your money stays where it is. You'll need a new code to sign back in."

    @Environment(AppEnvironment.self) private var environment
    let sections: [any ProfileSection.Type]

    @State private var confirmSignOut = false
    @State private var isSigningOut = false

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        VStack(spacing: MonacoTheme.Space.gutter) {
            SectionStack(context: (), sections: sections.map { $0.erased })
            Button("Sign out") {
                confirmSignOut = true
            }
            .buttonStyle(.monacoDestructive)
            .monacoFullWidthButtons()
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.m)
            .disabled(isSigningOut)
            .accessibilityIdentifier("profileSignOutButton")
            .confirmationDialog(Self.signOutTitle, isPresented: $confirmSignOut, titleVisibility: .visible) {
                Button("Sign out", role: .destructive) {
                    guard !isSigningOut else { return }
                    isSigningOut = true
                    Task {
                        await environment.signOut()
                        isSigningOut = false
                    }
                }
                .accessibilityIdentifier("profile-sign-out-confirm")
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(Self.signOutMessage)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
    }
}
