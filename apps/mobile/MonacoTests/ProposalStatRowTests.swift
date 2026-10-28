import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProposalStatRowTests {
    private nonisolated static let now = Date(timeIntervalSince1970: 1_792_662_780)
    private static var utc: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .current
        return calendar
    }

    private static func stats(_ fixture: Components.Schemas.ProposalDetail) -> ProposalStats {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"),
            transport: StubTransport(.failure(URLError(.notConnectedToInternet))))
        let model = ProposalDetailModel(
            sample: fixture, members: [], asset: .nvda, repository: ProposalsRepository(api: api),
            hints: FakeHintSource(), now: { now }, calendar: utc)
        return model.stats ?? ProposalStats(cells: [])
    }

    private static let open = stats(.openNVDA(now: now))

    @Test func theRowHasFourColumnsAtLargeTwoAtXXLargeAndOneAtAccessibility3() {
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .large) == 4)
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .xxLarge) == 2)
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .accessibility3) == 1)
    }

    @Test func eachCellReadsAsLabelCommaValueAtEverySize() async throws {
        for size in [DynamicTypeSize.large, .xxLarge, .accessibility3] {
            let labels = try await RenderedView.labels(ProposalStatRow(stats: Self.open), size: size) {
                $0.contains("Closes, 1:53 PM")
            }
            for expected in ["Price, $220.00", "Shares, ≈ 1.82", "Of treasury, 2%", "Closes, 1:53 PM"] {
                #expect(labels.contains(expected), "\(size): \(labels)")
            }
        }
    }

    @Test func theInfoGlyphStaysOffUntilTheMarketStatsSheetFlipsIt() {
        let hidden = ProposalStatRow.items(Self.open, showsInfo: false, onInfo: {})
        #expect(hidden.allSatisfy { $0.info == nil })
        let shown = ProposalStatRow.items(Self.open, showsInfo: true, onInfo: {})
        #expect(shown.map { $0.info != nil } == [true, false, false, false])
    }

    @Test func aFilledProposalColoursSinceInTheGainTone() {
        let items = ProposalStatRow.items(Self.stats(.filledNVDA(now: Self.now)), showsInfo: true, onInfo: {})
        #expect(items.map(\.value) == ["$219.40", "1.82", "$220.00", "+0.3%"])
        #expect(items.last?.tone == .gain)
        #expect(items.allSatisfy { $0.info == nil })
    }

    @Test func theHeroCrestLineReadsCabalProposerAndAge() {
        let line = ProposalHero.crestLine(
            cabalName: "AC Capital", proposer: "Mia", createdAt: Self.now.addingTimeInterval(-7_200), now: Self.now,
            calendar: Self.utc)
        #expect(line == "AC Capital · Mia proposed · 2h")
        let old = ProposalHero.crestLine(
            cabalName: "AC Capital", proposer: "Mia", createdAt: Self.now.addingTimeInterval(-86_400 * 3),
            now: Self.now, calendar: Self.utc)
        #expect(old == "AC Capital · Mia proposed · Oct 19")
    }

    @Test func theHeroAmountDropsWholeDollarCents() {
        #expect(ProposalHero.amountLabel(micros: 400_000_000) == "$400")
        #expect(ProposalHero.amountLabel(micros: 900_000_000) == "$900")
        #expect(ProposalHero.amountLabel(micros: 12_500_000) == "$12.50")
    }
}
