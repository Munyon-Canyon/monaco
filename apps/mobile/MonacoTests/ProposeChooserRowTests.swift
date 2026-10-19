import Testing

@testable import Monaco

@MainActor
struct ProposeChooserRowTests {
    @Test func buyAndSellUseTheV3ChooserGlyphs() {
        #expect(MonacoIcon.propose.symbol == "arrow.up.right")
        #expect(MonacoIcon.down.symbol == "arrow.down")
        #expect(MonacoIcon.bot.symbol == "cpu")
    }

    @Test func theBotChoiceStaysOffUntilTradingBotsLand() {
        #expect(ProposeChooserConfig.isBotProposalLive == false)
    }
}
