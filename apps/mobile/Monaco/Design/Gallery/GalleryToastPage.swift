#if DEBUG
import SwiftUI

struct GalleryToastPage: View {
    @State private var toast: MonacoToast?
    @State private var isLoadingBalance = false

    private let failure = "We couldn't confirm that went through. Check your balance before trying again."

    var body: some View {
        GalleryPage(title: "Toast") {
            GalleryGroup(title: "Panels") {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    GalleryCase(caption: "Success toast on the ink panel") {
                        MonacoToastBanner(message: "Added $50 to Weekend investors", isSuccess: true).monacoBleed()
                    }
                    GalleryCase(caption: "Error toast on the ink panel") {
                        MonacoToastBanner(message: failure).monacoBleed()
                    }
                    GalleryCase(caption: "Toast with an action") {
                        MonacoToastBanner(
                            message: "Left Weekend investors", action: MonacoToastAction(title: "Undo") {}
                        )
                        .monacoBleed()
                    }
                }
            }
            liveToasts
        }
        .monacoToast($toast, placement: .aboveBottomCTA)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button("Add money") {}.buttonStyle(.monacoPrimary)
            }
        }
    }

    private var liveToasts: some View {
        GalleryGroup(title: "Live toasts") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                Text("A toast stays up for as long as its message takes to read. Swipe down or tap to dismiss.")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                Button("Show success") {
                    toast = MonacoToast(message: "Added $50 to Weekend investors", isSuccess: true)
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
