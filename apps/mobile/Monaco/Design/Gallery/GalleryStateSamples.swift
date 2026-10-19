#if DEBUG
import SwiftUI

struct GalleryEmptySamples: View {
    var body: some View {
        GalleryGroup(title: "Empty and error states") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "EmptyState, text only") {
                    EmptyState(title: "Nothing bought yet", message: "Fund, then propose the first buy.")
                }
                GalleryCase(caption: "EmptyState, icon and one button") {
                    EmptyState(
                        title: "No cabals yet", message: "Start one with friends.", actionTitle: "Start a cabal",
                        systemImage: MonacoIcon.cabals.symbol, action: {})
                }
                GalleryCase(caption: "EmptyState, icon and two buttons") {
                    EmptyState(
                        title: "No cabals yet", message: "Start one with friends or find one to join.",
                        actionTitle: "Start a cabal", systemImage: MonacoIcon.cabals.symbol,
                        secondaryActionTitle: "Discover cabals", action: {}, secondaryAction: {})
                }
                GalleryCase(caption: "EmptyState.loadFailed") {
                    EmptyState.loadFailed(identifier: "gallery-empty-failed") {}
                }
                GalleryCase(caption: "MonacoErrorRow, notice") {
                    MonacoErrorRow(thing: "your holdings", identifier: "gallery-error-notice", inset: false) {}
                }
                GalleryCase(caption: "MonacoErrorRow, card") {
                    MonacoErrorRow(thing: "your holdings", identifier: "gallery-error-card", inset: false, style: .card)
                    {}
                }
            }
        }
    }
}

struct GalleryOutcomeSamples: View {
    var body: some View {
        GalleryGroup(title: "Outcome") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                GalleryCase(caption: "OutcomeView, done") {
                    OutcomeView(tone: .done, title: "Deposit received", message: "$50.00 is in your balance.")
                }
                GalleryCase(caption: "OutcomeView, pending") {
                    OutcomeView(tone: .pending, title: "Deposit on its way", message: "It can take a minute to arrive.")
                }
                GalleryCase(caption: "OutcomeView, error") {
                    OutcomeView(tone: .error, title: "Deposit failed", message: "Nothing left your balance.")
                }
            }
        }
    }
}

struct GallerySkeletonSamples: View {
    var body: some View {
        GalleryGroup(title: "Skeletons") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Title") { SkeletonTitle() }
                GalleryCase(caption: "Bar") { SkeletonBar() }
                GalleryCase(caption: "Card") { SkeletonCardBlock() }
                GalleryCase(caption: "Row") { MonacoRowSkeleton(rows: 2, markShape: .tile) }
            }
        }
    }
}
#endif
