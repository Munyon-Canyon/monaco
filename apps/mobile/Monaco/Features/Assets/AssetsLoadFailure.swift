import SwiftUI

struct AssetsLoadFailure: View {
    let identifier: String
    let retry: () -> Void

    var body: some View {
        EmptyState(
            title: "Couldn't load this view",
            message: "Your existing information hasn't been changed. Try again when your connection is available.",
            actionTitle: "Try again",
            actionIdentifier: identifier,
            systemImage: "wifi.exclamationmark",
            action: retry
        )
    }
}
