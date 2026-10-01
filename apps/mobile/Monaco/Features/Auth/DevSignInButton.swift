#if DEBUG
import SwiftUI

struct DevSignInButton: View {
    @Environment(AppEnvironment.self) private var environment
    private let session: DevSession?

    init(session: DevSession? = DevSession.fromLaunchEnvironment()) {
        self.session = session
    }

    var body: some View {
        if let session {
            Button("Dev: sign in as \(String(session.userID.prefix(8)))") {
                Task { await environment.signIn(dev: session) }
            }
            .accessibilityIdentifier("devSignInButton")
        }
    }
}
#endif
