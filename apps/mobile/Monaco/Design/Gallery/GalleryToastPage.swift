#if DEBUG
import SwiftUI

struct GalleryToastPage: View {
    @State private var toast: MonacoToast?
    @State private var isLoadingBalance = false

    private let failure = "We couldn't confirm that went through. Check your balance before trying again."

    var body: some View {
        GalleryPage(title: "Toast") {
            GalleryGroup(title: "Capsules") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    ForEach(ColorScheme.allCases, id: \.self) { scheme in
                        capsules.environment(\.colorScheme, scheme)
                            .padding(MonacoTheme.Space.m)
                            .background(MonacoTheme.canvas)
                    }
                }
            }
            liveToasts
        }
        .monacoToast($toast)
    }

    private var capsules: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            GalleryCase(caption: "An ordinary vote, with Undo") {
                MonacoToastBanner(
                    message: "Voted Yes. 6th of 8.", isSuccess: true, action: MonacoToastAction(title: "Undo") {},
                    insetsGutter: false)
            }
            GalleryCase(caption: "The vote that settles it, no Undo") {
                MonacoToastBanner(message: "Voted Yes. Passed 5–1.", isSuccess: true, insetsGutter: false)
            }
            GalleryCase(caption: "Failure") {
                MonacoToastBanner(message: failure, insetsGutter: false)
            }
            GalleryCase(caption: "Reduce Transparency on") {
                MonacoToastBanner(
                    message: "Voted Yes. 6th of 8.", isSuccess: true, action: MonacoToastAction(title: "Undo") {},
                    insetsGutter: false, opaque: true
                )
            }
        }
    }

    private var liveToasts: some View {
        GalleryGroup(title: "Live toasts") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                Text("A toast stays up for as long as its message takes to read. Swipe up to dismiss, or tap Undo.")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                Button("Show success") {
                    toast = MonacoToast(message: "Added $50 to Weekend investors", isSuccess: true)
                }
                .buttonStyle(.monacoSecondary)
                Button("Show a vote with Undo") {
                    toast = MonacoToast(
                        message: "Voted Yes. 6th of 8.", isSuccess: true,
                        action: MonacoToastAction(title: "Undo", expiresAt: Date().addingTimeInterval(15)) {})
                }
                .buttonStyle(.monacoSecondary)
                Button("Show a money failure") {
                    toast = MonacoToast(message: failure)
                }
                .buttonStyle(.monacoSecondary)
                Button("Show error while the balance reloads") {
                    toast = MonacoToast(message: "Couldn't load this. Pull down to try again")
                    isLoadingBalance.toggle()
                }
                .buttonStyle(.monacoSecondary)
                if isLoadingBalance {
                    ProgressView()
                        .tint(MonacoTheme.accent)
                        .frame(maxWidth: .infinity, minHeight: 96)
                } else {
                    MoneyText(micros: 1_248_500_000, style: .hero)
                        .frame(maxWidth: .infinity, minHeight: 96, alignment: .leading)
                }
            }
        }
    }
}
#endif
