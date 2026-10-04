import SwiftUI

struct ProposalCoinBurst: View {
    @State private var expanded = false

    var body: some View {
        ZStack {
            ForEach(0..<6, id: \.self) { index in
                Image(systemName: "circle.fill")
                    .foregroundStyle(MonacoTheme.warning)
                    .offset(x: expanded ? CGFloat(index - 3) * 18 : 0, y: expanded ? -28 : 0)
                    .opacity(expanded ? 0 : 1)
            }
        }
        .task {
            withAnimation(.easeOut(duration: 0.55)) { expanded = true }
        }
        .accessibilityHidden(true)
    }
}
