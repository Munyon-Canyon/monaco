import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit
import UniformTypeIdentifiers

@testable import Monaco

@MainActor
struct ShareScreenTests {
    private static let screenIdentifiers = [
        "share-screen", "share-card", "share-replay", "share-eye-pnl", "share-eye-trades", "share-eye-code",
        "share-format-post", "share-format-square", "share-format-story", "share-format-sticker", "share-send",
        "share-copy-image",
    ]

    private static func shown(_ sample: ShareSampleCase, expecting id: String) async throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(ShareSampleHarness(sample: sample))
        defer { TestWindow.close(window) }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }
        return Set(AccessibilityTree.identifiers(in: window))
    }

    @Test func theScreenCarriesEveryIdentifier() async throws {
        let ids = try await Self.shown(.screenPost, expecting: "share-card")

        for id in Self.screenIdentifiers { #expect(ids.contains(id), "\(id) in \(ids)") }
    }

    @Test(arguments: [ShareSampleCase.screenSquare, .screenStory, .screenSticker, .screenPnlOff, .screenReplay])
    func everyScreenCaseShowsTheCard(sample: ShareSampleCase) async throws {
        let ids = try await Self.shown(sample, expecting: "share-card")

        #expect(ids.contains("share-card"), "\(ids)")
        #expect(ids.contains("share-send"), "\(ids)")
    }

    @Test func theStickerFormatTitlesTheScreenSticker() {
        #expect(ShareSampleCase.screenSticker.screenTitle == "Sticker")
        #expect(ShareSampleCase.screenPost.screenTitle == "Share")
        #expect(ShareScreenModel.stickerTitle == "Sticker")
    }

    @Test(arguments: [ShareSampleCase.screenStake, .screenPortfolio, .stickerOff, .stickerOn, .stickerPassed])
    func theStakePortfolioAndStickerCasesRender(sample: ShareSampleCase) async throws {
        let ids = try await Self.shown(sample, expecting: sample.isScreen ? "share-card" : "share-sticker")

        #expect(ids.contains(sample.isScreen ? "share-card" : "share-sticker"), "\(ids)")
    }

    @Test func theCheckerboardAndStickerFitTheirBox() {
        let fitted = ShareScreenContent.fit(.sticker, in: CGSize(width: 360, height: 700))

        #expect(fitted.width == 360)
        #expect(abs(fitted.height - 204) < 0.0001)
    }

    @Test func theVoteScreenIsTitledShareVoteAndMarksTheUnusedEyes() async throws {
        let ids = try await Self.shown(.screenVote, expecting: "share-card")

        #expect(ShareSampleCase.screenVote.screenTitle == "Share vote")
        #expect(ShareSampleCase.screenPassed.screenTitle == "Share")
        #expect(ShareSampleCase.screenVote.unusedEyes == [.pnl, .trades])
        for id in Self.screenIdentifiers { #expect(ids.contains(id), "\(id) in \(ids)") }
    }

    @Test(arguments: [
        ShareSampleCase.passed, .passedPnlOn, .passedNoPassedPrice, .passedPending, .passedFailed, .passedNoLivePrice,
        .vote, .screenPassed,
    ])
    func everyProposalCaseBuildsACardAndRenders(sample: ShareSampleCase) throws {
        let spec = try #require(sample.spec)

        #expect(ShareImageExporter.render(spec, format: .square).size.width > 0)
    }

    @Test func theShareBSampleCarriesTheSealAndTheFill() throws {
        let spec = try #require(ShareSampleCase.passed.spec)

        #expect(spec.chart.markers.map(\.kind) == [.seal, .buy])
        #expect(spec.header.title == "Christelle called it")
    }

    @Test func theFigureCountUpLeavesAPassedTallyAlone() {
        #expect(ShareReplay.countUp("Passed 4\u{2013}1", progress: 0) == "Passed 4\u{2013}1")
    }

    @Test func loadingAndFailureHaveTheirOwnMarkersAndNoDock() async throws {
        let loading = try await Self.shown(.screenLoading, expecting: "share-loading")
        let failed = try await Self.shown(.screenFailed, expecting: "share-retry")

        #expect(loading.contains("share-loading"))
        #expect(!loading.contains("share-send"))
        #expect(failed.contains("share-retry"))
        #expect(!failed.contains("share-send"))
    }

    @Test func sharingBuildsAPayloadWithTheExportedImageAndTheCabalLink() throws {
        let spec = ShareSampleData.spec()
        let image = ShareImageExporter.render(spec, format: .post)
        let url = try #require(URL(string: "https://monacolabs.xyz/ANDRES?c=ac-capital"))

        let payload = ShareActions.payload(for: spec, url: url, image: image)

        #expect(payload.image === image)
        #expect(payload.url == url)
        #expect(URLComponents(url: payload.url, resolvingAgainstBaseURL: false)?.queryItems?.first?.name == "c")
        #expect(payload.title == "AC Capital \u{00B7} NVDA")
        let items = SystemShare.activityItems(for: payload)
        #expect(items.count == 2)
    }

    @Test func aCardWithoutAnInviteLinkSharesTheWebsite() {
        let spec = ShareSampleData.spec()
        let payload = ShareActions.payload(
            for: spec, url: nil, image: ShareImageExporter.render(spec, format: .post))

        #expect(payload.url == SharePayload.fallbackURL)
    }

    @Test func copyImagePutsPNGDataOnThePasteboard() throws {
        let name = UIPasteboard.Name("monaco-share-test-\(UUID().uuidString)")
        let pasteboard = try #require(UIPasteboard(name: name, create: true))
        defer { UIPasteboard.remove(withName: name) }
        let image = ShareImageExporter.render(ShareSampleData.spec(), format: .square)

        #expect(ShareActions.copy(image, to: pasteboard))

        let data = try #require(pasteboard.data(forPasteboardType: UTType.png.identifier))
        #expect(data == image.pngData())
        #expect(Array(data.prefix(4)) == [0x89, 0x50, 0x4E, 0x47])
        #expect(ShareActions.copiedMessage == "Image copied.")
    }

    @Test func eachEyeTogglesItsOwnFlag() {
        let all = ShareEyes()

        #expect(ShareEye.pnl.toggled(all) == ShareEyes(pnlDollars: false))
        #expect(ShareEye.trades.toggled(all) == ShareEyes(buySell: false))
        #expect(ShareEye.code.toggled(all) == ShareEyes(code: false))
        #expect(ShareEye.allCases.allSatisfy { $0.isOn(all) })
        #expect(ShareEye.pnl.toggled(ShareEye.pnl.toggled(all)) == all)
    }

    @Test func theFormatsCarryTheirLabelsAndIdentifiers() {
        #expect(ShareFormat.allCases.map(\.title) == ["Card 4:5", "Square", "Story", "Sticker"])
        #expect(
            ShareFormat.allCases.map(\.identifier)
                == ["share-format-post", "share-format-square", "share-format-story", "share-format-sticker"])
    }

    @Test func theReplayRunsForTwoPointFourSecondsAndClamps() {
        let start = Date(timeIntervalSince1970: 1_000)

        #expect(ShareReplay.duration == 2.4)
        #expect(ShareReplay.progress(from: start, at: start) == 0)
        #expect(abs(ShareReplay.progress(from: start, at: start.addingTimeInterval(1.2)) - 0.5) < 0.0001)
        #expect(ShareReplay.progress(from: start, at: start.addingTimeInterval(9)) == 1)
        #expect(ShareReplay.progress(from: start, at: start.addingTimeInterval(-1)) == 0)
    }

    @Test func theFigureCountsUpFromZeroToItself() {
        #expect(ShareReplay.countUp("+$1,027.90", progress: 0) == "+$0.00")
        #expect(ShareReplay.countUp("+$1,027.90", progress: 1) == "+$1,027.90")
        #expect(ShareReplay.countUp("+14.0%", progress: 0) == "+0.0%")
        #expect(ShareReplay.countUp("$400", progress: 0) == "$0")
        #expect(ShareReplay.countUp("No figure", progress: 0.5) == "No figure")
        let half = ShareReplay.countUp("+$1,027.90", progress: 0.5)
        #expect(half.hasPrefix("+$") && half != "+$1,027.90" && half != "+$0.00")
    }

    @Test func theReplaySpecChangesOnlyTheFigureUntilItFinishes() {
        let spec = ShareSampleData.spec()
        let mid = ShareReplay.spec(spec, at: 0.4)

        #expect(mid.figure.main != spec.figure.main)
        #expect(mid.header == spec.header)
        #expect(mid.chart == spec.chart)
        #expect(mid.stats == spec.stats)
        #expect(ShareReplay.spec(spec, at: 1) == spec)
    }

    @Test func thePreviewFitsEachFormatInsideItsBox() {
        let box = CGSize(width: 361, height: 700)

        #expect(ShareScreenContent.fit(.post, in: box) == CGSize(width: 361, height: 451.25))
        #expect(ShareScreenContent.fit(.square, in: box) == CGSize(width: 361, height: 361))
        let tall = ShareScreenContent.fit(.story, in: CGSize(width: 361, height: 500))
        #expect(tall.height == 500)
        #expect(abs(tall.width / tall.height - 1080.0 / 1920.0) < 0.0001)
        #expect(ShareScreenContent.fit(.story, in: box).width == 361)
    }
}
