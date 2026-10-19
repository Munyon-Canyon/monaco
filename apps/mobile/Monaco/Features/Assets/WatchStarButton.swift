import SwiftUI

struct WatchStarButton: View {
    let isWatched: Bool
    let addLabel: String
    let removeLabel: String
    let identifier: String
    let toggle: () -> Void

    var body: some View {
        MonacoGlassButton(
            systemImage: "star", accessibilityLabel: isWatched ? removeLabel : addLabel,
            style: isWatched ? .selected : .glass, action: toggle
        )
        .accessibilityAddTraits(isWatched ? .isSelected : [])
        .accessibilityIdentifier(identifier)
    }
}
