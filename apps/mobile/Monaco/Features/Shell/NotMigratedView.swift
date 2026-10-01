import SwiftUI

struct NotMigratedView: View {
    let screen: String

    var body: some View {
        Text("\(screen) isn't on the new backend yet.")
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .padding(MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
    }
}
