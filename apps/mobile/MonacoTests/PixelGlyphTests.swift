import SwiftUI
import Testing
import UIKit

@testable import Monaco

struct PixelBitmapTests {
    @Test func rowsParseLeftToRight() {
        var art = Array(repeating: String(repeating: ".", count: 16), count: 16)
        art[0] = "#..............#"
        art[15] = "........##......"
        let bitmap = PixelBitmap(art)
        #expect(bitmap.isLit(column: 0, row: 0))
        #expect(bitmap.isLit(column: 15, row: 0))
        #expect(!bitmap.isLit(column: 1, row: 0))
        #expect(bitmap.isLit(column: 8, row: 15))
        #expect(bitmap.isLit(column: 9, row: 15))
        #expect(bitmap.litCount == 4)
    }

    @Test func runsMergeAdjacentPixels() {
        var art = Array(repeating: String(repeating: ".", count: 16), count: 16)
        art[3] = "..###...##......"
        let runs = PixelBitmap(art).runs
        #expect(runs.count == 2)
        #expect(runs[0].row == 3 && runs[0].start == 2 && runs[0].length == 3)
        #expect(runs[1].row == 3 && runs[1].start == 8 && runs[1].length == 2)
    }

    @Test func subtractingCutsAHole() {
        let disc = PixelGlyphs.disc
        let cut = disc.subtracting(PixelGlyphs.check)
        #expect(cut.litCount == disc.litCount - PixelGlyphs.check.litCount)
        #expect(!cut.isLit(column: 13, row: 4))
    }

    @Test func flipsMirror() {
        let up = PixelGlyphs.up
        let down = up.flippedVertically
        #expect(down.isLit(column: 7, row: 13) == up.isLit(column: 7, row: 2))
        #expect(up.flippedHorizontally == up)
        #expect(PixelGlyphs.chevron.flippedHorizontally != PixelGlyphs.chevron)
    }

    @Test func unitSnapsToTheDevicePixel() {
        let rect = CGRect(x: 0, y: 0, width: 22, height: 22)
        let unit = PixelBitmap.unit(fitting: rect, scale: 3)
        #expect(unit == CGFloat(4) / 3)
        #expect(PixelBitmap.unit(fitting: CGRect(x: 0, y: 0, width: 1, height: 1), scale: 3) == CGFloat(1) / 3)
    }

    @Test func pathCoversEveryLitPixelAndNothingElse() {
        let bitmap = PixelGlyphs.check
        let path = bitmap.path(in: CGRect(x: 0, y: 0, width: 32, height: 32), scale: 2)
        for row in 0..<16 {
            for column in 0..<16 {
                let centre = CGPoint(x: CGFloat(column) * 2 + 1, y: CGFloat(row) * 2 + 1)
                #expect(path.contains(centre) == bitmap.isLit(column: column, row: row))
            }
        }
    }

    @Test func templateImageIsATemplateAtTheAskedSize() {
        let image = PixelGlyphs.home.templateImage(pointSize: 25, scale: 3)
        #expect(image.renderingMode == .alwaysTemplate)
        #expect(image.size == CGSize(width: 25, height: 25))
        #expect(image.scale == 3)
    }
}

struct PixelGlyphSetTests {
    @Test(arguments: MonacoIcon.allCases)
    func everyDrawnGlyphIsNonEmptyAndFitsTheGrid(icon: MonacoIcon) {
        guard let bitmap = icon.pixel() else { return }
        #expect(!bitmap.isEmpty)
        #expect(bitmap.litCount < 16 * 16)
        if let solid = icon.pixel(filled: true), solid != bitmap {
            #expect(solid.litCount >= bitmap.litCount)
        }
    }

    @Test func everyTabHasAnOutlineAndAFilledGlyph() {
        for icon in [MonacoIcon.home, .cabals, .markets, .feed, .profile] {
            #expect(icon.pixel(filled: false) != nil)
            #expect(icon.pixel(filled: true) != icon.pixel(filled: false))
            #expect(icon.tabImage(selected: true, scale: 3) != nil)
        }
    }

    @Test func systemNameLookupFindsDrawnIcons() {
        #expect(MonacoIcon(systemName: "house") == .home)
        #expect(MonacoIcon(systemName: "house.fill") == .home)
        #expect(MonacoIcon(systemName: "wifi.exclamationmark") == .offline)
        #expect(MonacoIcon(systemName: "no.such.symbol") == nil)
    }

    @Test func noTwoDrawnOutlinesAreTheSameExceptSharedArrows() {
        var seen: [PixelBitmap: MonacoIcon] = [:]
        let shared: Set<MonacoIcon> = [.deposit, .plus, .up, .expand, .down, .cashOut]
        for icon in MonacoIcon.allCases {
            guard let bitmap = icon.pixel() else { continue }
            if let other = seen[bitmap] {
                #expect(shared.contains(icon) && shared.contains(other), "\(icon) repeats \(other)")
            }
            seen[bitmap] = icon
        }
    }
}

struct CabalEmblemTests {
    @Test func emblemIsStableAndCaseInsensitive() {
        let id = "3f5b2c9e-8d1a-4e7f-9b6c-2a1d0e4f7c88"
        let emblem = CabalEmblem.forGroupId(id)
        #expect(CabalEmblem.forGroupId(id.uppercased()) == emblem)
        #expect(CabalEmblem.forGroupId(" \(id)\n") == emblem)
    }

    @Test func emblemsSpreadAcrossAllEight() {
        let used = Set((0..<200).map { CabalEmblem.forGroupId("group-\($0)") })
        #expect(used.count == CabalEmblem.allCases.count)
    }

    @Test func emblemIsNotTiedToTheTint() {
        let ids = (0..<200).map { "group-\($0)" }
        let pairs = Set(ids.map { "\(MonacoTheme.CabalTint.forGroupId($0))-\(CabalEmblem.forGroupId($0))" })
        #expect(pairs.count > CabalEmblem.allCases.count)
    }

    @Test(arguments: CabalEmblem.allCases)
    func everyEmblemIsDrawn(emblem: CabalEmblem) {
        #expect(emblem.bitmap.litCount > 40)
        #expect(emblem.bitmap.litCount < 200)
        #expect(!emblem.spokenName.isEmpty)
    }
}
