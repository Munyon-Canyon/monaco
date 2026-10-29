import MonacoCore
import UIKit

extension PushPrePrompt {
    func noteCabalJoined(after toasts: ToastCenter?) async {
        await noteCabalJoined()
    }

    func noteFirstVote(sample: PushSample, after toasts: ToastCenter?) async {
        while toasts?.current != nil || UIApplication.shared.hasPresentedSheet {
            do {
                try await Task.sleep(for: .milliseconds(100))
            } catch {
                return
            }
        }
        await noteFirstVote(sample: sample)
    }
}

extension UIApplication {
    fileprivate var hasPresentedSheet: Bool {
        let scenes = connectedScenes.compactMap { $0 as? UIWindowScene }
        return scenes.flatMap(\.windows).contains {
            $0.isKeyWindow && $0.rootViewController?.presentedViewController != nil
        }
    }
}
