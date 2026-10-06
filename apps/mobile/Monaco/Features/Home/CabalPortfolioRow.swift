import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalPortfolioRow: View {
    private struct Figures {
        let valueMicros: Int64
        let returnBps: Int64?
    }

    private let id: String
    private let name: String
    private let pictureURL: String?
    private let subtitle: String
    private let figures: Figures?
    private let isLast: Bool

    init(cabal: Components.Schemas.MyCabal, isLast: Bool = false) {
        id = cabal.id
        name = cabal.name
        pictureURL = cabal.pictureUrl
        subtitle = Self.members(cabal.memberCount)
        figures = nil
        self.isLast = isLast
    }

    init(row: PortfolioSummary.Row, isLast: Bool = false) {
        id = row.id
        name = row.name
        pictureURL = row.pictureURL
        subtitle = row.share
        figures = Figures(valueMicros: row.valueMicros, returnBps: row.returnBps)
        self.isLast = isLast
    }

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: id))) {
            MonacoRow(title: name, subtitle: subtitle, isLast: isLast) {
                CabalMark(groupId: id, name: name, size: 40, pictureUrl: pictureURL)
            } trailing: {
                if let figures {
                    VStack(alignment: .trailing, spacing: 2) {
                        MoneyText(micros: figures.valueMicros, style: .row)
                        if let bps = figures.returnBps {
                            PercentText(basisPoints: bps, style: .caption)
                        } else {
                            PercentText(percentReturn: nil, style: .caption)
                        }
                    }
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-row-\(id)")
    }

    static func members(_ count: Int32) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }
}
