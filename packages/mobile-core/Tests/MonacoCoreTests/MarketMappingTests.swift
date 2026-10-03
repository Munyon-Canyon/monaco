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
        XCTAssertNil(asset.status.nextSession)
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
        session: Components.Schemas.MarketSession
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
            session: session
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
