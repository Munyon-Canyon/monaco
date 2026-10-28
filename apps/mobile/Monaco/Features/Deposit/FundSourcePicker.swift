import MonacoCore
import SwiftUI

struct FundSourcePicker: View {
    let rows: [FundSourceRow]
    let selectedID: String
    let onPick: (String) -> Void

    var body: some View {
        VStack(spacing: 0) {
            ForEach(rows) { row in
                Button {
                    onPick(row.id)
                } label: {
                    MonacoRow(
                        title: row.name, subtitle: row.ownership,
                        leading: {
                            CabalMark(
                                groupId: row.id, name: row.name, size: MonacoRowLayout.baseMarkSize,
                                pictureUrl: row.pictureURL)
                        },
                        trailing: {
                            if row.id == selectedID {
                                Image(systemName: "checkmark")
                                    .font(MonacoTheme.Typo.subhead)
                                    .foregroundStyle(MonacoTheme.brand)
                                    .accessibilityHidden(true)
                            }
                        })
                }
                .buttonStyle(.monacoRow)
                .accessibilityAddTraits(row.id == selectedID ? .isSelected : [])
                .accessibilityIdentifier("fund-source-\(row.id)")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .monacoFittedSheet(title: FundSourceCopy.title, showsClose: true)
    }
}
