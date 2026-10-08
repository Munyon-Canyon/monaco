import SwiftUI

struct FirstRunAction {
    let title: String
    let identifier: String
    let isDisabled: Bool
    let perform: () -> Void
}

struct FirstRunToolbar: ToolbarContent {
    let signOut: FirstRunAction?
    let skip: FirstRunAction?

    var body: some ToolbarContent {
        if let signOut {
            ToolbarItem(placement: .topBarLeading) { button(signOut) }
                .withoutGlass()
        }
        if let skip {
            ToolbarItem(placement: .topBarTrailing) { button(skip) }
                .withoutGlass()
        }
    }

    private func button(_ action: FirstRunAction) -> some View {
        Button(action.title, action: action.perform)
            .lineLimit(1)
            .fixedSize()
            .buttonStyle(.monacoText)
            .disabled(action.isDisabled)
            .accessibilityIdentifier(action.identifier)
    }
}

extension ToolbarContent {
    @ToolbarContentBuilder
    fileprivate func withoutGlass() -> some ToolbarContent {
        if #available(iOS 26.0, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}
