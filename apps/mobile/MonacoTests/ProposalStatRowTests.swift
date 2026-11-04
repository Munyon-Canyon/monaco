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

    private static func model(_ fixture: Components.Schemas.ProposalDetail) -> ProposalDetailModel {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"),
            transport: StubTransport(.failure(URLError(.notConnectedToInternet))))
        return ProposalDetailModel(
            sample: fixture, members: [], asset: .nvda, repository: ProposalsRepository(api: api),
            hints: FakeHintSource(), now: { now }, calendar: utc)
    }

    private static func stats(_ fixture: Components.Schemas.ProposalDetail) -> ProposalStats {
        model(fixture).stats ?? ProposalStats(cells: [])
    }

    private static let open = stats(.openNVDA(now: now))

    @Test func theRowHasFourColumnsAtLargeTwoAtXXLargeAndOneAtAccessibility3() {
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .large) == 4)
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .xxLarge) == 2)
        #expect(StatMetrics.columns(requested: 4, dynamicTypeSize: .accessibility3) == 1)
    }

    @Test func aFarOutCloseKeepsItsDayAndTimeWholeAndSpeaksWithPlainSpaces() {
        let cells = [ProposalStats.Cell(label: "Closes", value: "Fri 5:41 PM")]
        let items = ProposalStatRow.items(ProposalStats(cells: cells))
        #expect(items.first?.value == "Fri 5:41\u{00A0}PM")
        #expect(items.first?.spokenValue == "Fri 5:41 PM")
    }

    private static let spoken = "About 1.82 shares at $220.00. 2 percent of treasury."

    @Test func theOpenHeroReadsItsFactsAsOneLineAtEverySize() async throws {
        let model = Self.model(.openNVDA(now: Self.now))
        let detail = try #require(model.value)
        let summary = try #require(model.summary)
        let facts = ProposalFacts.make(detail: detail, asset: model.asset)
        #expect(facts?.spoken == Self.spoken)
        for size in [DynamicTypeSize.large, .accessibility3] {
            let hero = NavigationStack {
                ProposalHero(summary: summary, asset: model.asset, members: [], paused: false, facts: facts)
            }
            let labels = try await RenderedView.labels(hero, size: size) { $0.contains(Self.spoken) }
            #expect(labels.contains(Self.spoken), "\(size): \(labels)")
        }
    }

    @Test func aFilledProposalHasNoFactsSoTheSlotDrawsTheGrid() throws {
        let model = Self.model(.filledNVDA(now: Self.now))
        let detail = try #require(model.value)
        #expect(ProposalFacts.make(detail: detail, asset: model.asset) == nil)
    }

    @Test func theInfoLabelNamesWhatTheSheetExplains() {
        #expect(ProposalHero.infoLabel(.price) == "About price")
        #expect(ProposalHero.infoLabel(.estimate) == "About the estimate")
    }

    @Test func aFilledProposalColoursSinceInTheGainTone() {
        let items = ProposalStatRow.items(Self.stats(.filledNVDA(now: Self.now)))
        #expect(items.map(\.value) == ["$219.40", "1.82", "$220.00", "+0.3%"])
        #expect(items.last?.tone == .gain)
        #expect(items.allSatisfy { $0.info == nil })
    }

    @Test func theProposerFirstNameIsTheFirstWordOrNil() {
        #expect(ProposalHero.firstName(of: "Mia Chen") == "Mia")
        #expect(ProposalHero.firstName(of: "Mia") == "Mia")
        #expect(ProposalHero.firstName(of: "  Mia   Chen ") == "Mia")
        #expect(ProposalHero.firstName(of: "  ") == nil)
        #expect(ProposalHero.firstName(of: nil) == nil)
    }

    @Test func theProposerGroupReadsRelativeThenDated() {
        let recent = ProposalHero.proposedLabel(
            firstName: "Mia", createdAt: Self.now.addingTimeInterval(-7_200), now: Self.now, calendar: Self.utc)
        #expect(recent == "Mia proposed this 2 hours ago")
        let minutes = ProposalHero.proposedLabel(
            firstName: "Mia", createdAt: Self.now.addingTimeInterval(-60), now: Self.now, calendar: Self.utc)
        #expect(minutes == "Mia proposed this 1 minute ago")
        let old = ProposalHero.proposedLabel(
            firstName: "Mia", createdAt: Self.now.addingTimeInterval(-86_400 * 3), now: Self.now, calendar: Self.utc)
        #expect(old == "Mia proposed this on Oct 19")
    }

    @Test func theHeroAmountDropsWholeDollarCents() {
        #expect(ProposalHero.amountLabel(micros: 400_000_000) == "$400")
        #expect(ProposalHero.amountLabel(micros: 900_000_000) == "$900")
        #expect(ProposalHero.amountLabel(micros: 12_500_000) == "$12.50")
    }
}
