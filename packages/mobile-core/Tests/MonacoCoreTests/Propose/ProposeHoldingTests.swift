import MonacoAPI
import MonacoCore
import XCTest

final class ProposeHoldingTests: XCTestCase {
    func testSellTokenAmountFor25And50PercentAndAll() {
        let apple = Self.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678, valueMicros: 278_470_000)

        XCTAssertEqual(apple.tokenAmount(forMicros: 69_610_000), 30_083_178)
        XCTAssertEqual(apple.tokenAmount(forMicros: 139_230_000), 60_170_678)
        XCTAssertEqual(apple.tokenAmount(forMicros: 278_470_000), 120_345_678)
        XCTAssertEqual(apple.tokenAmount(forMicros: 0), 0)
    }

    func testAllRoundedDownToTheCentStillSellsEveryToken() {
        let apple = Self.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678, valueMicros: 278_476_543)

        XCTAssertEqual(apple.tokenAmount(forMicros: 278_470_000), 120_345_678)
        XCTAssertLessThan(apple.tokenAmount(forMicros: 278_460_000), 120_345_678)
    }

    func testChooserDetailNamesTwoHoldingsAndCountsTheRest() {
        let apple = Self.holding(name: "Apple")
        let nvidia = Self.holding(name: "Nvidia")
        let tesla = Self.holding(name: "Tesla")

        XCTAssertEqual(ProposeHolding.chooserDetail([]), "Nothing to sell yet")
        XCTAssertEqual(ProposeHolding.chooserDetail([apple]), "Apple")
        XCTAssertEqual(ProposeHolding.chooserDetail([apple, nvidia]), "Apple and Nvidia")
        XCTAssertEqual(ProposeHolding.chooserDetail([apple, nvidia, tesla]), "Apple, Nvidia and 1 more")
    }

    func testQuantityCountsSharesOrTokensFromTheDisplayedUnits() {
        let apple = Self.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let spacex = Self.holding(name: "SpaceX", kind: .preIpo, units: "1.0000", tokenAmount: 1_000_000_000)

        XCTAssertEqual(apple.quantity(of: 60_172_839), "0.6017 shares")
        XCTAssertEqual(apple.quantity(of: 0), "0 shares")
        XCTAssertEqual(spacex.quantity(of: 1_000_000_000), "1 token")
        XCTAssertEqual(spacex.quantity(of: 500_000_000), "0.5 tokens")
    }

    func testDetailNamesTheStockAndTheShareCount() {
        let apple = Self.holding(name: "Apple", units: "0.7300", tokenAmount: 73_000_000)

        XCTAssertEqual(apple.detail, "Apple · 0.73 shares")
    }

    func testHelperSaysWhatTheCabalHoldsInSharesAndDollars() {
        let apple = Self.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678, valueMicros: 278_470_000)
        let unpriced = Self.holding(name: "Apple", valueMicros: 0)

        XCTAssertEqual(apple.helperText, "The cabal holds 1.2034 shares · $278.47")
        XCTAssertEqual(unpriced.helperText, "No price for this stock right now")
    }

    func testReadsTheTickerWithoutTheTokenSuffix() {
        XCTAssertEqual(Self.holding(name: "Apple").ticker, "AAPL")
    }

    static func holding(
        name: String, kind: Components.Schemas.CabalHolding.KindPayload = .equity, units: String = "1.0000",
        tokenAmount: Int64 = 100_000_000, valueMicros: Int64 = 100_000_000
    ) -> ProposeHolding {
        ProposeHolding(
            Components.Schemas.CabalHolding(
                symbol: name == "Apple" ? "AAPLx" : name.uppercased() + (kind == .preIpo ? "" : "x"),
                displayName: name, kind: kind,
                units: units, tokenAmount: tokenAmount, priceMicros: 231_400_000, valueMicros: valueMicros,
                weightBps: 5000, costBasisMicros: valueMicros, pnlMicros: 0))
    }
}
