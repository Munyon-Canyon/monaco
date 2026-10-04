import Foundation
import MonacoCore
import Testing

@testable import Monaco

private typealias CabalsTabModel = Monaco.CabalsTabModel

/// Holds a fake read open until the test lets it finish, one caller at a time
/// and in arrival order. Interleavings are then the test's to choose, rather
/// than whatever order the main actor happens to run its tasks in.
@MainActor
private final class CallGate {
    private var waiters: [CheckedContinuation<Void, Never>] = []
    private var banked = 0

    func wait() async {
        if banked > 0 {
            banked -= 1
            return
        }
        await withCheckedContinuation { waiters.append($0) }
    }

    /// Lets the longest-waiting caller through. Banked if nobody is waiting yet.
    func release() {
        if waiters.isEmpty {
            banked += 1
            return
        }
        waiters.removeFirst().resume()
    }
}

@MainActor
private final class RecordingDataSource: CabalsTabDataSource {
    var pnlRanges: [GroupPnLRange] = []
    /// Reads that have come back. `pnlRanges` records arrival, this records exit.
    var pnlCompletions = 0
    var pnlError: Error?
    var boardRows: [GroupLeaderboardRowDTO] = []
    var boardLoads = 0
    /// Points per cabal, per range, for the chart tests.
    var seriesByRange: [GroupPnLRange: [GroupPnLSeriesDTO]] = [:]
    /// Held open so a test can drive what happens while a chart read is in flight.
    var pnlGate: (() async -> Void)?

    func leaderboard() async throws -> GroupLeaderboardResponseDTO {
        boardLoads += 1
        return GroupLeaderboardResponseDTO(groups: boardRows)
    }

    func pnlHistory(range: GroupPnLRange) async throws -> MyGroupsPnLHistoryDTO {
        pnlRanges.append(range)
        if let pnlGate { await pnlGate() }
        pnlCompletions += 1
        if let pnlError { throw pnlError }
        return MyGroupsPnLHistoryDTO(range: range.rawValue, series: seriesByRange[range] ?? [])
    }
}

private func sampleSeries(id: String, points: Int) -> GroupPnLSeriesDTO {
    let now = Date()
    return GroupPnLSeriesDTO(
        groupID: id,
        name: "Cabal \(id)",
        range: GroupPnLRange.oneMonth.rawValue,
        points: (0..<points).map { index in
            GroupPnLPointDTO(
                at: now.addingTimeInterval(TimeInterval(-3600 * (points - index))),
                potValueUsd: "100.00",
                netInUsd: "100.00",
                dollarPnl: "+1.00"
            )
        }
    )
}

@MainActor
struct CabalsTabModelTests {

    @Test func changingRangeReloadsTheChart() async throws {
        let source = RecordingDataSource()
        let model = make(source)

        model.selectRange(.threeMonths)
        try await waitUntil("the chart to load") { source.pnlRanges == [.threeMonths] && model.loadingRange == nil }

        #expect(source.pnlRanges == [.threeMonths])
        #expect(model.range == .threeMonths)
    }

    // MARK: - P&L chart (#294)

    @Test func aRangeWithThinHistoryKeepsTheChartSection() async throws {
        // Arrange: 1M has two full lines, 1D has a single point per cabal —
        // exactly what a quiet day looks like for young cabals.
        let source = RecordingDataSource()
        source.seriesByRange = [
            .oneMonth: [sampleSeries(id: "a", points: 8), sampleSeries(id: "b", points: 6)],
            .oneDay: [sampleSeries(id: "a", points: 1), sampleSeries(id: "b", points: 1)],
        ]
        let model = make(source)

        // Act
        await model.reload()
        #expect(CabalsTabModel.isChartable(model.series))
        model.selectRange(.oneDay)
        try await waitUntil("the day chart to load") { model.seriesRange == .oneDay && model.loadingRange == nil }

        // Assert: the day itself has nothing to draw, but the section — and so
        // the range picker that gets you back out — has earned its place. The
        // section rule itself is asserted, not just the flags behind it.
        #expect(!CabalsTabModel.isChartable(model.series))
        #expect(model.hasChartableHistory)
        #expect(model.showsChartSection(hasCabals: true))
        #expect(!model.showsChartSection(hasCabals: false))
    }

    @Test func aSupersededRangeLoadDoesNotClearTheNewerSpinner() async throws {
        // Arrange: only 3M has drawable history, so if the superseded 1D load
        // were the one that landed, the series would be empty. The gate makes
        // the interleaving the test's choice rather than the scheduler's.
        let source = RecordingDataSource()
        source.seriesByRange = [
            .threeMonths: [sampleSeries(id: "a", points: 8), sampleSeries(id: "b", points: 6)]
        ]
        let gate = CallGate()
        source.pnlGate = { await gate.wait() }
        let model = make(source)

        // Act: pick a range, then pick another before the first can land.
        model.selectRange(.oneDay)
        try await waitUntil("the 1D read to start") { source.pnlRanges == [.oneDay] }
        model.selectRange(.threeMonths)
        try await waitUntil("the 3M read to start") { source.pnlRanges == [.oneDay, .threeMonths] }

        gate.release()
        try await waitUntil("the superseded 1D read to return") { source.pnlCompletions == 1 }
        gate.release()
        try await waitUntil("the live 3M read to return") { source.pnlCompletions == 2 }
        try await waitUntil("the chart to settle") { model.loadingRange == nil }

        // Assert: each load knew its own range, the newer one won, and the
        // superseded one neither cleared the spinner nor left it stuck on.
        #expect(model.range == .threeMonths)
        #expect(model.seriesRange == .threeMonths)
        #expect(CabalsTabModel.isChartable(model.series))
    }

    @Test func aSupersededLoadOfTheSameRangeDoesNotClearTheNewerSpinner() async throws {
        // Arrange: the cold start every member gets. `.task` calls `reload()`,
        // then `/v1/home` lands and `onChange(of: joinedIDs)` calls it again —
        // two loads, same range, overlapping. Tracking the load by its *range*
        // could not tell them apart, so the first to exit (the cancelled one,
        // because `defer` runs on the guard-return path too) cleared the live
        // load's spinner and the chart section dropped out mid-load.
        let source = RecordingDataSource()
        source.seriesByRange = [
            .oneMonth: [sampleSeries(id: "a", points: 8), sampleSeries(id: "b", points: 6)]
        ]
        let gate = CallGate()
        source.pnlGate = { await gate.wait() }
        let model = make(source)

        // Act
        let first = Task { await model.reload() }
        try await waitUntil("the first chart read to start") { source.pnlRanges.count == 1 }
        let second = Task { await model.reload() }  // cancel-and-replace
        try await waitUntil("the second chart read to start") { source.pnlRanges.count == 2 }
        #expect(source.pnlRanges == [.oneMonth, .oneMonth])

        // The superseded load is the first to come back.
        gate.release()
        await first.value

        // Assert: it wrote nothing, and the load still running owns the spinner.
        #expect(model.series.isEmpty)
        #expect(model.loadingRange == .oneMonth)
        #expect(model.isChartLoading)
        #expect(model.showsChartSection(hasCabals: true))

        gate.release()
        await second.value

        #expect(model.loadingRange == nil)
        #expect(!model.isChartLoading)
        #expect(CabalsTabModel.isChartable(model.series))
    }

    @Test func aFailedRangeSwitchDropsLinesThatBelongToTheOldRange() async throws {
        // Arrange: 1M is drawn.
        let source = RecordingDataSource()
        source.seriesByRange = [
            .oneMonth: [sampleSeries(id: "a", points: 8), sampleSeries(id: "b", points: 6)]
        ]
        let model = make(source)
        await model.reload()
        #expect(model.seriesRange == .oneMonth)

        // Act: tap 1D and have the read fail.
        source.pnlError = Monaco.MonacoAPIError.httpStatus(500)
        model.selectRange(.oneDay)
        try await waitUntil("the failed chart") { model.chartFailed && model.loadingRange == nil }

        // Assert: the 1D chip is highlighted, so the 1M lines cannot still be
        // on screen at full opacity with no error. The section — and the picker
        // that gets the member back to 1M — stays.
        #expect(model.chartFailed)
        #expect(model.series.isEmpty)
        #expect(model.seriesRange == nil)
        #expect(model.showsChartSection(hasCabals: true))
    }

    @Test func aFailedRefreshOfTheRangeOnScreenKeepsItsLines() async throws {
        // A failed refresh is not a failed switch: the lines still belong to the
        // highlighted chip, so keep them rather than blanking a good chart.
        let source = RecordingDataSource()
        source.seriesByRange = [
            .oneMonth: [sampleSeries(id: "a", points: 8), sampleSeries(id: "b", points: 6)]
        ]
        let model = make(source)
        await model.reload()

        source.pnlError = Monaco.MonacoAPIError.httpStatus(500)
        await model.loadChart()

        #expect(model.seriesRange == .oneMonth)
        #expect(CabalsTabModel.isChartable(model.series))
    }
}

extension CabalsTabModelTests {
    fileprivate func make(_ source: RecordingDataSource) -> CabalsTabModel {
        CabalsTabModel(dataSource: source)
    }

    fileprivate func waitUntil(_ description: String, _ condition: () -> Bool) async throws {
        let start = ContinuousClock.now
        while !condition() {
            if ContinuousClock.now - start > .seconds(2) {
                Issue.record("timed out waiting for \(description)")
                return
            }
            await Task.yield()
        }
    }
}
