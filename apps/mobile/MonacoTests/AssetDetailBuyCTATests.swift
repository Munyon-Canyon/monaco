import Testing

@testable import Monaco

struct AssetDetailBuyCTATests {
    @Test func untradableAssetsShowTheDisabledCopy() {
        #expect(AssetDetailBuyCTA.title(tradable: false) == "Can't buy right now")
    }

    @Test func tradableAssetsOpenTheBuyProposalInStocks() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "AAPLx") { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "AAPLx", kind: .buy))
        #expect(opened?.tab == .stocks)
    }
}
