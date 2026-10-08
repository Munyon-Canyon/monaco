import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

struct HoldingsMixColorTests {
    @Test func cashIsVisibleOnEverySurfaceInBothSchemes() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            for surface in [MonacoTheme.canvas, MonacoTheme.surface, MonacoTheme.surfaceSunken] {
                let ratio = WCAGContrast.ratio(MonacoTheme.cashFill, on: [surface], scheme)
                #expect(ratio >= 3, "cash fill \(ratio):1 in scheme \(scheme.rawValue)")
            }
        }
    }

    @Test func cashIsToldApartFromEveryStockStepOnTheCanvasForEveryCabalColour() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            var canvas = WCAGContrast.resolve(MonacoTheme.canvas, scheme)
            canvas.alpha = 1
            let cash = Self.oklab(
                WCAGContrast.composite(WCAGContrast.resolve(MonacoTheme.cashFill, scheme), over: canvas))
            for cabalTint in MonacoTheme.CabalTint.allCases {
                for step in 0..<CabalPotSummary.Swatch.steps {
                    let color = AllocationPalette.color(for: .stock(step: step), tint: cabalTint)
                    let stock = Self.oklab(WCAGContrast.composite(WCAGContrast.resolve(color, scheme), over: canvas))
                    #expect(Self.distance(cash, stock) > 0.04, "\(cabalTint) step \(step) scheme \(scheme.rawValue)")
                }
            }
        }
    }

    private static func oklab(_ rgba: WCAGContrast.RGBA) -> (Double, Double, Double) {
        func linear(_ v: Double) -> Double { v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4) }
        let r = linear(rgba.red)
        let g = linear(rgba.green)
        let b = linear(rgba.blue)
        let l = cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
        let m = cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
        let s = cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
        return (
            0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s,
            1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s,
            0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s
        )
    }

    private static func distance(_ a: (Double, Double, Double), _ b: (Double, Double, Double)) -> Double {
        let (dl, da, db) = (a.0 - b.0, a.1 - b.1, a.2 - b.2)
        return (dl * dl + da * da + db * db).squareRoot()
    }

    @Test func stockStepsFadeInOrderAndPastTheLadderShareTheLast() {
        let opacities = (0..<CabalPotSummary.Swatch.steps).map(AllocationPalette.opacity(forStep:))
        #expect(opacities == opacities.sorted(by: >))
        #expect(Set(opacities).count == opacities.count)
        #expect(AllocationPalette.opacity(forStep: 99) == opacities.last)
        #expect(CabalPotSummary.Swatch.forStock(at: 99) == .stock(step: CabalPotSummary.Swatch.steps - 1))
    }
}
