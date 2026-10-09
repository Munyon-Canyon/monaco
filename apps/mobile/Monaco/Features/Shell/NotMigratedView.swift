import SwiftUI

struct NotMigratedView: View {
    let screen: String

    var body: some View {
        EmptyState(
            title: "\(screen) is on its way",
            message: "This part of Monaco isn't ready yet.",
            isOnlyContent: true
        )
        .monacoCanvas()
    }
}
