import Observation
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct PresentedSheetsTests {
    @Test(.timeLimit(.minutes(1)))
    func closingARealSheetResetsItsBinding() async throws {
        let flag = SheetFlag()
        let (window, root) = try Self.window(hosting: SheetHost(flag: flag))
        defer { window.isHidden = true }
        flag.isPresented = true
        await Self.until { flag.contentShown }

        await PresentedSheets.dismiss(from: root)

        #expect(root.presentedViewController == nil)
        await Self.until { !flag.isPresented }
        #expect(flag.isPresented == false)
    }

    @Test(.timeLimit(.minutes(1)))
    func withNothingPresentedItReturnsAtOnce() async throws {
        let (window, root) = try Self.window(hosting: SheetHost(flag: SheetFlag()))
        defer { window.isHidden = true }

        await PresentedSheets.dismiss(from: root)

        #expect(root.presentedViewController == nil)
    }

    private static func window(hosting view: some View) throws -> (UIWindow, UIViewController) {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let root = UIHostingController(rootView: view)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = root
        window.isHidden = false
        return (window, root)
    }

    private static func until(_ predicate: () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { continuation in
                withObservationTracking {
                    _ = predicate()
                } onChange: {
                    continuation.resume()
                }
            }
        }
    }
}

@Observable
@MainActor
private final class SheetFlag {
    var isPresented = false
    var contentShown = false
}

private struct SheetHost: View {
    @Bindable var flag: SheetFlag

    var body: some View {
        Color.clear.sheet(isPresented: $flag.isPresented) {
            Text("Sheet").onAppear { flag.contentShown = true }
        }
    }
}
