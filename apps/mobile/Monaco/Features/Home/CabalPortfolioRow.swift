import MonacoCore
import SwiftUI

struct CabalPortfolioRow: View {
    private let id: String
    private let name: String
    private let pictureURL: String?
    private let subtitle: String?
    private let valueMicros: Int64
    private let returnBps: Int64?
    private let isLast: Bool

    init(row: PortfolioSummary.Row, isLast: Bool = false) {
        id = row.id
        name = row.name
        pictureURL = row.pictureURL
        subtitle = row.share
        valueMicros = row.valueMicros
        returnBps = row.returnBps
        self.isLast = isLast
    }

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: id))) {
            MonacoRow(title: name, subtitle: subtitle, isLast: isLast) {
                CabalMark(groupId: id, name: name, pictureUrl: pictureURL)
            } trailing: {
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    MoneyText(micros: valueMicros, style: .row)
                    if let returnBps {
                        PercentText(basisPoints: returnBps, style: .caption)
                    } else {
                        PercentText(percentReturn: nil, style: .caption)
                    }
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-row-\(id)")
    }
}
