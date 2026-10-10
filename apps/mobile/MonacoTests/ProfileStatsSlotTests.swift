import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ProfileStatsSlotTests {
    @Test func theStatsAndCabalsSlotsAreLive() {
        #expect(ProfileStatsSlot.isLive)
        #expect(ProfileCabalsSlot.isLive)
    }

    @Test func theBandStacksAtAccessibilitySizesOnly() {
        #expect(ProfileStatsBand.isStacked(.accessibility3))
        #expect(ProfileStatsBand.isStacked(.accessibility1))
        #expect(!ProfileStatsBand.isStacked(.xxxLarge))
        #expect(!ProfileStatsBand.isStacked(.large))
    }

    private func height(of view: some View, width: CGFloat) -> CGFloat {
        UIHostingController(rootView: view.dynamicTypeSize(.accessibility5))
            .sizeThatFits(in: CGSize(width: width, height: CGFloat.greatestFiniteMagnitude)).height
    }

    private func column(_ label: String, _ value: String, _ detail: String?) -> ProfileStatColumn {
        ProfileStatColumn(value: value, detail: detail, label: label, tone: nil, stacked: true)
    }

    @Test func aStackedFigureStaysOnOneLine() {
        for (value, detail) in [("$1,000.00", nil), ("+$14.00", "+1.4%")] as [(String, String?)] {
            let oneLine = height(of: column("", "$1", detail).figures(alignment: .trailing), width: 160)
            let figure = height(of: column("", value, detail).figures(alignment: .trailing), width: 160)
            #expect(figure <= oneLine + 2, "\(value) broke onto a second line")
        }
    }

    @Test func aStackedRowPutsTheLabelWholeAboveTheFigure() {
        let column = column("In cabals", "$1,000.00", nil)
        let label = height(of: column.labelText, width: 375)
        let figure = height(of: column.figures(alignment: .trailing), width: 375)
        let row = height(of: column, width: 375)
        let stacked = label + MonacoTheme.Space.xs + figure
        #expect(abs(row - stacked) <= 2, "the row did not stack the label over the figure")
    }
}
