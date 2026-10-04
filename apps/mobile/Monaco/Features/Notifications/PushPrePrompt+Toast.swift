import MonacoCore

extension PushPrePrompt {
    func noteCabalJoined(after toasts: ToastCenter?) async {
        while toasts?.current != nil {
            do {
                try await Task.sleep(for: .milliseconds(100))
            } catch {
                return
            }
        }
        await noteCabalJoined()
    }
}
