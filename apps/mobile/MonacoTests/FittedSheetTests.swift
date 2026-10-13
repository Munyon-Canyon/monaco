import Foundation
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
@Suite(.serialized)
struct FittedSheetTests {
    @Test(.timeLimit(.minutes(1)))
    func theNewCabalSheetShowsItsTitleAndStartRowWhole() async throws {
        try await Self.expectWhole(
            "new-cabal-create-row", titled: "New cabal", in: AnyView(NewCabalSheet(onCreate: {})))
    }

    @Test(.timeLimit(.minutes(1)))
    func theFacePickerShowsItsPhotoButtonWhole() async throws {
        let sheet = FacePickerSheet(currentAnimal: nil, onPickAnimal: { _ in }, onPickPhoto: { _ in })
        try await Self.expectWhole("face-choose-photo", titled: "Your face", in: AnyView(sheet))
    }

    @Test(.timeLimit(.minutes(1)))
    func thePushPrePromptShowsItsButtonsWhole() async throws {
        let prompt = PushPrePrompt(
            authorization: UndecidedAuthorization(),
            defaults: UserDefaults(suiteName: "fitted-sheet-tests") ?? .standard,
            register: {})
        try await Self.expectWhole(
            "push-pre-prompt-turn-on", titled: "Know when your cabal votes and trades",
            in: AnyView(PushPrePromptSheet(prompt: prompt)))
    }

    private static func expectWhole(_ identifier: String, titled title: String, in sheet: AnyView) async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let root = UIHostingController(rootView: Color.clear.sheet(isPresented: .constant(true)) { sheet })
        let window = UIWindow(windowScene: scene)
        window.rootViewController = root
        window.makeKeyAndVisible()
        defer { window.isHidden = true }
        try #require(await eventually { root.presentedViewController != nil }, "the sheet never opened")
        let presented = try #require(root.presentedViewController)

        var report = "never found \(identifier)"
        let whole = await eventually {
            guard let visible = visibleRect(of: presented) else { return false }
            guard let row = element(identifier, in: presented.view),
                let heading = element(labelled: title, in: presented.view)
            else { return false }
            report =
                "\(identifier) at \(row.accessibilityFrame), title at \(heading.accessibilityFrame), "
                + "sheet shows \(visible)"
            return visible.contains(row.accessibilityFrame) && visible.contains(heading.accessibilityFrame)
        }
        #expect(whole, "\(report)")
        presented.dismiss(animated: false)
    }

    private static func visibleRect(of controller: UIViewController) -> CGRect? {
        guard let view = controller.viewIfLoaded else { return nil }
        let bounds = view.bounds.inset(by: UIEdgeInsets(top: 0, left: 0, bottom: view.safeAreaInsets.bottom, right: 0))
        return view.convert(bounds, to: nil).insetBy(dx: -0.5, dy: -0.5)
    }

    private static func element(_ identifier: String, in root: NSObject) -> NSObject? {
        first(in: root) { node in
            node.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier))
                && node.value(forKey: "accessibilityIdentifier") as? String == identifier
        }
    }

    private static func element(labelled label: String, in root: NSObject) -> NSObject? {
        first(in: root) { $0.accessibilityLabel == label && $0.accessibilityTraits.contains(.header) }
    }

    private static func first(in root: NSObject, depth: Int = 0, where match: (NSObject) -> Bool) -> NSObject? {
        if match(root) { return root }
        guard depth < 80 else { return nil }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        for child in children {
            if let found = first(in: child, depth: depth + 1, where: match) { return found }
        }
        return nil
    }

    private static func eventually(_ predicate: () -> Bool) async -> Bool {
        for _ in 0..<20_000 where !predicate() {
            await nextRunLoopTurn()
        }
        return predicate()
    }
}

private struct UndecidedAuthorization: NotificationAuthorizing {
    func status() async -> PushAuthorization { .notDetermined }
    func request() async -> Bool { false }
}

@MainActor
private func nextRunLoopTurn() async {
    await withCheckedContinuation { continuation in
        RunLoop.main.perform { continuation.resume() }
    }
}
