import MonacoRanking
import SwiftUI

struct CallBoardInfoSheet: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(CallBoardCopy.infoTitle)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            ForEach(CallBoardCopy.infoLines, id: \.self) { line in
                Text(line)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(MonacoTheme.Space.gutter)
        .monacoCanvas()
        .presentationDetents([.medium])
        .accessibilityIdentifier("leaderboard-info-sheet")
    }
}
