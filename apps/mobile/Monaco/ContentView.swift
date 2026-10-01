import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var auth: PrivyAuthService

    var body: some View {
        root
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoRootAppearance()
            .onAppear { MonacoLaunchTrace.markFirstFrame() }
    }

    @ViewBuilder
    private var root: some View {
        #if DEBUG
        if let screen = debugHarnessRoot(auth: auth) {
            screen
        } else {
            AuthGateView(auth: auth)
        }
        #else
        AuthGateView(auth: auth)
        #endif
    }
}

#Preview {
    ContentView()
        .environmentObject(PrivyAuthService())
}
