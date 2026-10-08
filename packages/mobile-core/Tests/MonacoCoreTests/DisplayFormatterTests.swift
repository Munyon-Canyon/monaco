import XCTest

@testable import MonacoCore

final class DisplayFormatterTests: XCTestCase {
    func testPercentReturnFormatter_positiveReturn_showsPlusPrefix() {
        // Arrange
        let raw = "0.124"

        // Act
        let formatted = PercentReturnFormatter.format(raw)

        // Assert
        XCTAssertTrue(formatted.hasPrefix("+"))
        XCTAssertTrue(formatted.contains("%"))
    }

    func testPercentReturnFormatter_negativeRatio_formatsAsPercent() {
        // Backend sends signed ratios for losses, e.g. formatPercentReturnDecimal(-0.036).
        XCTAssertEqual(PercentReturnFormatter.format("-0.036"), "\u{2212}3.6%")
        XCTAssertEqual(PercentReturnFormatter.format("0"), "0.0%")
        XCTAssertEqual(PercentReturnFormatter.format("+0.124"), "+12.4%")
        XCTAssertEqual(PercentReturnFormatter.format("+12.4%"), "+12.4%")
    }

    func testPercentReturnFormatter_nilPercentReturn_showsEmDashOrHidden() {
        // Arrange
        let raw: String? = nil

        // Act
        let formatted = PercentReturnFormatter.format(raw)

        // Assert
        XCTAssertEqual(formatted, "—")
    }

    func testDollarPnlFormatter_negativeShowsLossCopy() {
        // Arrange
        let raw = "-$12.40"

        // Act
        let formatted = DollarPnlFormatter.format(raw)

        // Assert
        XCTAssertTrue(formatted.localizedCaseInsensitiveContains("loss"))
    }

    func testSlicePercentFormatter_formatsOneDecimal() {
        // Arrange
        let raw = "0.425"

        // Act
        let formatted = SlicePercentFormatter.format(raw)

        // Assert
        XCTAssertEqual(formatted, "42.5%")
    }

    func testUsdAmountFormatter_formatsDecimalStringAsCurrency() {
        XCTAssertEqual(UsdAmountFormatter.format(decimalString: "2100.05"), "$2,100.05")
    }

    func testUsdAmountFormatter_formatsMicrosAsCurrency() {
        XCTAssertEqual(UsdAmountFormatter.format(micros: 2_100_050_000), "$2,100.05")
    }

    func testUsdAmountFormatter_flooredMicrosNeverReadsMoreThanMaxFills() {
        XCTAssertEqual(UsdAmountFormatter.format(flooredMicros: 2_999_999), "$2.99")
        XCTAssertEqual(UsdAmountFormatter.format(flooredMicros: 1_000_000), "$1.00")
        XCTAssertEqual(UsdAmountFormatter.flooredToCents(2_999_999), 2_990_000)
    }

    func testStakeWithdrawConverter_shareMicros_scalesWithUsdTarget() {
        let shares = StakeWithdrawConverter.shareMicros(
            forUsdMicros: 1_050_025_000,
            totalEquityUsdMicros: 2_100_050_000,
            maxShareMicros: 2_100_050
        )
        XCTAssertEqual(shares, 1_050_025)
    }

    func testStakeWithdrawConverter_fullWithdraw_returnsMaxShares() {
        XCTAssertTrue(
            StakeWithdrawConverter.isFullWithdraw(
                selectedUsdMicros: 2_100_050_000,
                totalEquityUsdMicros: 2_100_050_000
            )
        )
        XCTAssertEqual(
            StakeWithdrawConverter.shareMicros(
                forUsdMicros: 2_100_050_000,
                totalEquityUsdMicros: 2_100_050_000,
                maxShareMicros: 2_100_050
            ),
            2_100_050
        )
    }

    func testStakeWithdrawConverter_usdMicrosForFraction_atMax_returnsFullEquity() {
        XCTAssertEqual(
            StakeWithdrawConverter.usdMicros(forFraction: 1, maxUsdMicros: 2_100_050_000),
            2_100_050_000
        )
    }

    // MARK: - Demo polish formatters

    func testPercentReturnFormatter_usesTypographicMinusAndUnsignedZero() {
        XCTAssertEqual(PercentReturnFormatter.format("-0.036"), "\u{2212}3.6%")
        XCTAssertEqual(PercentReturnFormatter.format("0.096"), "+9.6%")
        XCTAssertEqual(PercentReturnFormatter.format("-0.0001"), "0.0%")
        XCTAssertEqual(PercentReturnFormatter.format("-0"), "0.0%")
        XCTAssertEqual(PercentReturnFormatter.format("-3.6%"), "\u{2212}3.6%")
        XCTAssertEqual(PercentReturnFormatter.format("\u{2212}0.036"), "\u{2212}3.6%")
        XCTAssertEqual(PercentReturnFormatter.format(""), "—")
        XCTAssertEqual(PercentReturnFormatter.format(nil), "—")
        XCTAssertEqual(PercentReturnFormatter.format("—"), "—")
    }

    func testUsdAmountFormatter_negativeAmountReadsLikeEveryOtherNegativeFigure() {
        // Never "$-12.50": the sign goes first, and it is U+2212, as `compact` already does.
        XCTAssertEqual(UsdAmountFormatter.format(decimal: Decimal(string: "-12.50")!), "\u{2212}$12.50")
        XCTAssertEqual(UsdAmountFormatter.format(micros: -12_431_800_000), "\u{2212}$12,431.80")
        XCTAssertEqual(UsdAmountFormatter.format(decimalString: "-1234.5"), "\u{2212}$1,234.50")
        // Dust that rounds to nothing keeps the unsigned zero.
        XCTAssertEqual(UsdAmountFormatter.format(decimal: Decimal(string: "-0.001")!), "$0.00")
        XCTAssertEqual(UsdAmountFormatter.format(decimal: Decimal(string: "12.50")!), "$12.50")
    }

    func testPercentFormatters_rejectValuesThatAreNotNumbers() {
        // A NaN or infinite ratio from the API must read as no figure, not "+nan%".
        // Both of them: returning the raw string just moved the garbage, so a slice read
        // "nan" where the return next to it read "—".
        for raw in ["nan", "-nan", "inf", "-infinity"] {
            XCTAssertEqual(PercentReturnFormatter.format(raw), "—", raw)
            XCTAssertEqual(SlicePercentFormatter.format(raw), "—", raw)
        }
    }

    func testDollarPnlFormatter_readsATypographicMinusAsALoss() {
        XCTAssertEqual(DollarPnlFormatter.format("\u{2212}$3.10"), "\u{2212}$3.10 loss")
    }

    func testUsdAmountFormatter_compact() {
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "12431.8"), "$12,431.80")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "99999.99"), "$99,999.99")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "100000"), "$100K")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "124500"), "$124.5K")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "12431180"), "$12.4M")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "1951100000000"), "$2T")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "1500000000000"), "$1.5T")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "2000000000"), "$2B")
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "0"), "$0.00")
    }

    func testProposalShareFormatter_sharesLabel() {
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "120340000"), "1.2034 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "120345678"), "1.2035 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "50000000"), "0.5 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "100000000"), "1 share")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "300000000"), "3 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "0"), "0 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "1000"), "< 0.0001 shares")
        XCTAssertEqual(ProposalShareFormatter.sharesLabel(fromAtomics: "garbage"), "garbage")
    }

    func testRelativeTimeFormatter_label() {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        let now = ISO8601DateFormatter().date(from: "2026-09-19T12:00:00Z")!
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-19T11:59:30Z", now: now, calendar: calendar), "now")
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-19T11:45:00Z", now: now, calendar: calendar), "15m")
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-19T08:59:00.500Z", now: now, calendar: calendar), "3h")
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-14T09:00:00Z", now: now, calendar: calendar), "Sep 14")
        XCTAssertEqual(
            RelativeTimeFormatter.label(iso: "2025-12-31T09:00:00Z", now: now, calendar: calendar), "Dec 31, 2025")
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-19T12:05:00Z", now: now, calendar: calendar), "now")
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "not a date", now: now, calendar: calendar), "")
    }

    func testRelativeTimeFormatter_datesUseLocalCalendarDay() {
        var tokyo = Calendar(identifier: .gregorian)
        tokyo.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        let now = ISO8601DateFormatter().date(from: "2026-09-19T12:00:00Z")!
        // 20:00 UTC on Sep 14 is already Sep 15 in Tokyo.
        XCTAssertEqual(RelativeTimeFormatter.label(iso: "2026-09-14T20:00:00Z", now: now, calendar: tokyo), "Sep 15")
    }

    func testUsdAmountFormatter_signedMicros() {
        XCTAssertEqual(UsdAmountFormatter.format(signedMicros: 48_200_000), "+$48.20")
        XCTAssertEqual(UsdAmountFormatter.format(signedMicros: -7_600_000), "\u{2212}$7.60")
        XCTAssertEqual(UsdAmountFormatter.format(signedMicros: 0), "$0.00")
        XCTAssertEqual(UsdAmountFormatter.format(signedMicros: 1), "$0.00")
        XCTAssertEqual(UsdAmountFormatter.format(signedMicros: -1), "$0.00")
    }

    func testPercentFormatter_basisPoints() {
        XCTAssertEqual(PercentFormatter.format(basisPoints: 1234, signed: true), "+12.34%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: -5, signed: true), "\u{2212}0.05%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: 0, signed: true), "0.00%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: 1234, signed: false), "12.34%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: 123_456, signed: true), "+1,234.56%")
    }

    func testPercentFormatter_largeBasisPointsAndIntMin() {
        XCTAssertEqual(
            PercentFormatter.format(basisPoints: Int64.min, signed: true), "\u{2212}92,233,720,368,547,758.08%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: Int64.max, signed: true), "+92,233,720,368,547,758.07%")
    }

    func testDollarPnlFormatter_positivePassesThroughUnchanged() {
        XCTAssertEqual(DollarPnlFormatter.format("+$12.40"), "+$12.40")
        XCTAssertEqual(DollarPnlFormatter.format("$0.00"), "$0.00")
    }

    func testUsdAmountFormatter_decimalStringFallsBackToRawOnUnparseable() {
        XCTAssertEqual(UsdAmountFormatter.format(decimalString: "not-a-number"), "$not-a-number")
    }

    func testUsdAmountFormatter_compactFallsBackToRawOnUnparseable() {
        XCTAssertEqual(UsdAmountFormatter.compact(decimalString: "garbage"), "$garbage")
    }

    func testStakeWithdrawConverter_usdMicrosFromDecimalString() {
        XCTAssertEqual(StakeWithdrawConverter.usdMicros(fromDecimalString: "12.50"), 12_500_000)
        XCTAssertEqual(StakeWithdrawConverter.usdMicros(fromDecimalString: "0"), 0)
        XCTAssertNil(StakeWithdrawConverter.usdMicros(fromDecimalString: "-1.00"))
        XCTAssertNil(StakeWithdrawConverter.usdMicros(fromDecimalString: "not a number"))
    }

    func testStakeWithdrawConverter_fractionForUsdMicros() {
        XCTAssertEqual(StakeWithdrawConverter.fraction(forUsdMicros: 50, maxUsdMicros: 100), 0.5)
        XCTAssertEqual(StakeWithdrawConverter.fraction(forUsdMicros: -10, maxUsdMicros: 100), 0)
        XCTAssertEqual(StakeWithdrawConverter.fraction(forUsdMicros: 500, maxUsdMicros: 100), 1)
        XCTAssertEqual(StakeWithdrawConverter.fraction(forUsdMicros: 50, maxUsdMicros: 0), 0)
    }

    func testAssetCatalogDisplayName_preIpoFallsBackToSymbolWhenNameIsEmpty() {
        XCTAssertEqual(
            AssetCatalogDisplayName.format(catalogName: "", symbol: "PREX", kind: .preIpo),
            "PREX"
        )
    }

    func testAssetCatalogDisplayName_stockUsesStrippedCatalogNameWhenNotInTable() {
        XCTAssertEqual(
            AssetCatalogDisplayName.format(catalogName: "Zyxwv xStock", symbol: "ZYXWV", kind: .stock),
            "Zyxwv"
        )
    }

    func testAssetCatalogDisplayName_stockFallsBackToSymbolWhenCatalogNameIsEmpty() {
        XCTAssertEqual(
            AssetCatalogDisplayName.format(catalogName: "", symbol: "ZYXWV", kind: .stock),
            "ZYXWV"
        )
    }
}
