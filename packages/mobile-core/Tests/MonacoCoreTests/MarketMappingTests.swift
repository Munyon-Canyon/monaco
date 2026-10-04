import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class MarketMappingTests: XCTestCase {
    private let sampledAt = Date(timeIntervalSince1970: 1_772_596_200)
    private let nextBell = Date(timeIntervalSince1970: 1_772_619_600)

    func testPricedEquityMapsNamePriceChangeAndSparkline() {
        let asset = MarketMapping.asset(
            summary(
                symbol: "AAPLx",
                displayName: "Apple xStock",
                issuer: .xstocks,
                kind: .equity,
                logoUrl: "https://cdn.example.com/AAPLx.png",
                priceMicros: 110_000_000,
                priceAsOf: sampledAt,
                changeBps: 1_000,
                sparklineMicros: [100_000_000, 110_000_000],
                session: session(state: .open, nextState: .afterHours, nextTransition: nextBell)
            )
        )

        XCTAssertEqual(asset.symbol, "AAPLx")
        XCTAssertEqual(asset.ticker, "AAPL")
        XCTAssertEqual(asset.name, "Apple")
        XCTAssertEqual(asset.issuer, .xstocks)
        XCTAssertEqual(asset.kind, .stock)
        XCTAssertEqual(asset.logoURL, URL(string: "https://cdn.example.com/AAPLx.png"))
        XCTAssertEqual(asset.priceMicros, 110_000_000)
        XCTAssertEqual(asset.priceText, "$110.00")
        XCTAssertEqual(asset.changeBasisPoints, 1_000)
        XCTAssertEqual(asset.changeText, "+10.00%")
        XCTAssertEqual(asset.sparkline?.heights.count, 2)
        XCTAssertEqual(asset.sparkline?.firstUsdcMicros, 100_000_000)
        XCTAssertEqual(asset.sparkline?.lastUsdcMicros, 110_000_000)
        XCTAssertEqual(asset.session, .open)
        XCTAssertTrue(asset.showsSessionChip)
        XCTAssertTrue(asset.status.isOpen)
        XCTAssertFalse(asset.status.afterHours)
        XCTAssertEqual(asset.status.nextSession, .afterHours)
        XCTAssertEqual(asset.status.nextTransition, nextBell)
        XCTAssertFalse(asset.status.earlyClose)
        XCTAssertNil(asset.status.holiday)
        XCTAssertTrue(asset.isTradable)
    }

    func testUnpricedAssetRendersEmDashAndNoSparkline() {
        let asset = MarketMapping.asset(
            summary(
                symbol: "NEWCO",
                displayName: "Newco",
                issuer: .xstocks,
                kind: .equity,
                logoUrl: nil,
                priceMicros: nil,
                priceAsOf: nil,
                changeBps: nil,
                sparklineMicros: nil,
                session: session(state: .closed)
            )
        )

        XCTAssertNil(asset.priceMicros)
        XCTAssertEqual(asset.priceText, "—")
        XCTAssertNil(asset.changeBasisPoints)
        XCTAssertEqual(asset.changeText, "—")
        XCTAssertNil(asset.sparkline)
        XCTAssertNil(asset.logoURL)
        XCTAssertEqual(asset.session, .closed)
        XCTAssertFalse(asset.status.isOpen)
        XCTAssertTrue(asset.status.afterHours)
    }

    func testUntradableAssetStaysInTheCatalog() {
        let asset = MarketMapping.asset(
            summary(
                symbol: "AMBRx", displayName: "Amber", issuer: .xstocks, kind: .equity, logoUrl: nil,
                priceMicros: nil, priceAsOf: nil, changeBps: nil, sparklineMicros: nil,
                session: session(state: .closed), tradable: false
            ))

        XCTAssertFalse(asset.isTradable)
    }

    func testNegativeChangeAndBlankLogoStayReadable() {
        let asset = MarketMapping.asset(
            summary(
                symbol: "MSFTx",
                displayName: "   ",
                issuer: .xstocks,
                kind: .equity,
                logoUrl: "not a url",
                priceMicros: 1_500_000_000,
                priceAsOf: sampledAt,
                changeBps: -50,
                sparklineMicros: [10],
                session: session(state: .preMarket, nextState: .open, nextTransition: nextBell)
            )
        )

        XCTAssertEqual(asset.ticker, "MSFT")
        XCTAssertEqual(asset.name, "MSFT")
        XCTAssertNil(asset.logoURL)
        XCTAssertEqual(asset.priceText, "$1,500.00")
        XCTAssertEqual(asset.changeText, "−0.50%")
        XCTAssertNil(asset.sparkline)
        XCTAssertEqual(asset.session, .preMarket)
        XCTAssertEqual(asset.status.nextSession, .open)
        XCTAssertFalse(asset.status.isOpen)
        XCTAssertTrue(asset.status.afterHours)
    }

    func testPreIPOUsesTheServerNameAndHidesTheSessionChip() {
        let asset = MarketMapping.asset(
            summary(
                symbol: "tSpaceX",
                displayName: "T-SpaceX",
                issuer: .tessera,
                kind: .preIpo,
                logoUrl: nil,
                priceMicros: 42_000_000,
                priceAsOf: sampledAt,
                changeBps: 0,
                sparklineMicros: [42_000_000, 42_000_000],
                session: session(state: .open, continuous: true)
            )
        )

        XCTAssertEqual(asset.ticker, "tSpaceX")
        XCTAssertEqual(asset.name, "SpaceX")
        XCTAssertEqual(asset.issuer, .tessera)
        XCTAssertEqual(asset.kind, .preIpo)
        XCTAssertEqual(asset.changeText, "0.00%")
        XCTAssertEqual(asset.sparkline?.heights, [0.5, 0.5])
        XCTAssertFalse(asset.showsSessionChip)
        XCTAssertEqual(asset.session, .open)
        XCTAssertFalse(asset.status.isOpen)
        XCTAssertFalse(asset.status.afterHours)
        XCTAssertTrue(asset.isTradable)
        XCTAssertNil(asset.status.nextSession)
    }

}

extension MarketMappingTests {
    func testPreIPOWithSiblingListingsMapsTheDetailWithoutLosingItsCatalogFields() {
        let detail = MarketMapping.detail(
            .init(
                symbol: "tSpaceX",
                displayName: "T-SpaceX",
                issuer: .tessera,
                kind: .preIpo,
                logoUrl: nil,
                priceMicros: 42_000_000,
                priceAsOf: sampledAt,
                changeBps: 125,
                sparklineMicros: [40_000_000, 42_000_000],
                session: .init(state: .open, continuous: true, holiday: "", earlyClose: false),
                decimals: 6,
                uiMultiplier: .init(num: 1, den: 1),
                tradable: true,
                otherListings: [
                    .init(
                        symbol: "SPACEX",
                        displayName: "SpaceX",
                        issuer: .prestocks,
                        kind: .preIpo,
                        tradable: false
                    )
                ],
                attribution: "Data provided by CoinGecko"
            )
        )

        XCTAssertEqual(detail.asset.symbol, "tSpaceX")
        XCTAssertEqual(detail.asset.ticker, "tSpaceX")
        XCTAssertEqual(detail.asset.name, "SpaceX")
        XCTAssertEqual(detail.asset.issuer, .tessera)
        XCTAssertEqual(detail.asset.kind, .preIpo)
        XCTAssertEqual(detail.asset.priceText, "$42.00")
        XCTAssertEqual(detail.asset.changeText, "+1.25%")
        XCTAssertEqual(detail.asset.sparkline?.lastUsdcMicros, 42_000_000)
        XCTAssertFalse(detail.asset.showsSessionChip)
        XCTAssertEqual(detail.otherListings.count, 1)
        XCTAssertEqual(detail.otherListings[0].symbol, "SPACEX")
        XCTAssertEqual(detail.otherListings[0].name, "SpaceX")
        XCTAssertEqual(detail.otherListings[0].ticker, "SPACEX")
        XCTAssertEqual(detail.otherListings[0].issuer, .prestocks)
        XCTAssertEqual(detail.otherListings[0].kind, .preIpo)
        XCTAssertFalse(detail.otherListings[0].isTradable)
    }

    func testEachSessionState() {
        XCTAssertEqual(MarketMapping.session(session(state: .preMarket)), .preMarket)
        XCTAssertEqual(MarketMapping.session(session(state: .open)), .open)
        XCTAssertEqual(MarketMapping.session(session(state: .afterHours)), .afterHours)
        XCTAssertEqual(MarketMapping.session(session(state: .closed)), .closed)

        let holiday = MarketMapping.status(
            session(state: .closed, holiday: "Thanksgiving Day", nextState: .preMarket, nextTransition: nextBell),
            session: .closed
        )
        XCTAssertEqual(holiday.holiday, "Thanksgiving Day")
        XCTAssertEqual(holiday.nextSession, .preMarket)
        XCTAssertEqual(holiday.session, .closed)

        let early = MarketMapping.status(
            session(state: .open, earlyClose: true, nextState: .afterHours, nextTransition: nextBell),
            session: .open
        )
        XCTAssertTrue(early.earlyClose)
        XCTAssertTrue(early.isOpen)
        XCTAssertEqual(early.nextSession, .afterHours)

        let evening = MarketMapping.status(
            session(state: .afterHours, nextState: .closed, nextTransition: nextBell),
            session: .afterHours
        )
        XCTAssertEqual(evening.nextSession, .closed)
        XCTAssertTrue(evening.afterHours)
        XCTAssertFalse(evening.isOpen)
    }

    func testPageKeepsTheCursorAndRowOrder() {
        let page = MarketMapping.page(
            Components.Schemas.AssetList(
                assets: [
                    summary(
                        symbol: "AAPLx",
                        displayName: "Apple xStock",
                        issuer: .xstocks,
                        kind: .equity,
                        logoUrl: nil,
                        priceMicros: 1,
                        priceAsOf: sampledAt,
                        changeBps: nil,
                        sparklineMicros: nil,
                        session: session(state: .open)
                    ),
                    summary(
                        symbol: "TSLAx",
                        displayName: "Tesla xStock",
                        issuer: .prestocks,
                        kind: .equity,
                        logoUrl: nil,
                        priceMicros: nil,
                        priceAsOf: nil,
                        changeBps: nil,
                        sparklineMicros: nil,
                        session: session(state: .closed)
                    ),
                ],
                nextCursor: "cursor-2"
            )
        )

        XCTAssertEqual(page.assets.map(\.symbol), ["AAPLx", "TSLAx"])
        XCTAssertEqual(page.assets.map(\.issuer), [.xstocks, .prestocks])
        XCTAssertEqual(page.nextCursor, "cursor-2")
    }

    private func summary(
        symbol: String,
        displayName: String,
        issuer: Components.Schemas.AssetIssuer,
        kind: Components.Schemas.AssetKind,
        logoUrl: String?,
        priceMicros: Int64?,
        priceAsOf: Date?,
        changeBps: Int32?,
        sparklineMicros: [Int64]?,
        session: Components.Schemas.MarketSession,
        tradable: Bool = true
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            symbol: symbol,
            displayName: displayName,
            issuer: issuer,
            kind: kind,
            logoUrl: logoUrl,
            priceMicros: priceMicros,
            priceAsOf: priceAsOf,
            changeBps: changeBps,
            sparklineMicros: sparklineMicros,
            session: session,
            tradable: tradable
        )
    }

    private func session(
        state: Components.Schemas.MarketState,
        continuous: Bool = false,
        holiday: String = "",
        earlyClose: Bool = false,
        nextState: Components.Schemas.MarketSession.NextStatePayload? = nil,
        nextTransition: Date? = nil
    ) -> Components.Schemas.MarketSession {
        Components.Schemas.MarketSession(
            state: state,
            continuous: continuous,
            holiday: holiday,
            earlyClose: earlyClose,
            nextState: nextState,
            nextTransition: nextTransition
        )
    }
}
