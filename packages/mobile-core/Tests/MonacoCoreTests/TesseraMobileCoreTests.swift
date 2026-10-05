import XCTest

@testable import MonacoCore

final class TesseraMobileCoreTests: XCTestCase {
    func testAssetSymbolFormatter_preIpo_keepsTSpaceX() {
        XCTAssertEqual(AssetSymbolFormatter.display("tSpaceX", kind: .preIpo), "tSpaceX")
    }

    func testDisplayName_preIpo_stripsTPrefix() {
        XCTAssertEqual(CatalogAssetNameFormatter.format("T-SpaceX", kind: .preIpo), "SpaceX")
        XCTAssertEqual(
            AssetCatalogDisplayName.format(catalogName: "T-SpaceX", symbol: "tSpaceX", kind: .preIpo),
            "SpaceX"
        )
    }

    func testTokenQuantity_nineDecimals_preIpo_labelsTokens() {
        XCTAssertEqual(
            TokenQuantityFormatter.label(fromAtomics: "1500000000", decimals: 9, kind: .preIpo),
            "1.5 tokens"
        )
        XCTAssertEqual(
            TokenQuantityFormatter.label(fromAtomics: "1000000000", decimals: 9, kind: .preIpo),
            "1 token"
        )
    }

    func testMainFlowCopyAudit_preIpoStrings_pass() {
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(PreIpoCopy.auditedStrings))
    }
}
