import MonacoCore
import SwiftUI

struct ProposeReviewScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: MonacoCore.ProposeReviewModel?
    @State private var cabalFailed = false
    private let service: MonacoCore.ProposeService
    private let cabalID: String
    private let draft: MonacoCore.ProposalDraft
    private let preview: MonacoCore.ProposePreview
    private let trade: MonacoCore.ProposeTrade

    init(
        service: MonacoCore.ProposeService, cabalID: String, draft: MonacoCore.ProposalDraft,
        preview: MonacoCore.ProposePreview, trade: MonacoCore.ProposeTrade
    ) {
        self.service = service
        self.cabalID = cabalID
        self.draft = draft
        self.preview = preview
        self.trade = trade
    }

    var body: some View {
        Group {
            if let model {
                content(model)
            } else if cabalFailed {
                MonacoErrorRow(thing: "this cabal", identifier: "propose-review-error") { Task { await load() } }
            } else {
                ProposeReviewSkeleton()
            }
        }
        .monacoCanvas()
        .navigationTitle("Review")
        .navigationBarTitleDisplayMode(.inline)
        .task { await load() }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-review-screen")
    }

    private func content(_ model: MonacoCore.ProposeReviewModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                ProposeReviewHeader(model: model)
                MonacoGroupedList {
                    ForEach(Array(model.rows.enumerated()), id: \.offset) { index, row in
                        ReceiptLine(
                            label: row.label, value: Self.value(row), isLast: index == model.rows.count - 1)
                    }
                }
                if let reason = model.reason {
                    ProposalReasonSection(title: model.reasonTitle, text: reason)
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .safeAreaInset(edge: .bottom) {
            VStack(spacing: 0) {
                Text(model.voteCaption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.sm)
                    .background(MonacoTheme.canvas)
                    .accessibilityIdentifier("propose-review-vote-caption")
                BottomCTA {
                    Button(model.sendTitle) { Task { await send(model) } }
                        .buttonStyle(.monacoPrimary)
                        .disabled(model.isSending)
                        .accessibilityIdentifier("propose-review-send")
                }
            }
        }
    }

    private static func value(_ row: MonacoCore.ProposeReviewModel.Row) -> ReceiptLine.Value {
        ["Price", "Cabal gets", "Cabal keeps"].contains(row.label)
            ? .data(row.value) : .words(row.value)
    }

    private func load() async {
        cabalFailed = false
        do {
            let cabal = try await MonacoCore.ProposeCabalInfo.load(api: environment.api, cabalID: cabalID)
            model = MonacoCore.ProposeReviewModel(
                service: service, cabalID: cabalID, cabal: cabal, draft: draft, preview: preview, trade: trade)
        } catch {
            cabalFailed = true
        }
    }

    private func send(_ model: MonacoCore.ProposeReviewModel) async {
        await model.send()
        if model.proposalID != nil {
            toasts.show(success: model.successToast)
            environment.navigator.closeProposeFlow()
        } else if let message = model.errorMessage {
            toasts.current = MonacoToast(message: message)
        }
    }
}

private struct ProposeReviewHeader: View {
    let model: MonacoCore.ProposeReviewModel

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(model.operation)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(model.isSell ? MonacoTheme.lossOnWash : MonacoTheme.profitOnWash)
                .padding(.horizontal, MonacoTheme.Space.sm)
                .padding(.vertical, MonacoTheme.Space.xs)
                .background(
                    model.isSell ? MonacoTheme.lossWash : MonacoTheme.profitWash, in: Capsule())
            Text(model.heroAmount)
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.ink)
                .minimumScaleFactor(0.6)
                .lineLimit(1)
            Text("of \(model.assetName)")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(2)
            Text(model.destination)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(2)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(model.title + " " + model.destination)
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("propose-review-header")
    }
}

private struct ProposeReviewSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            SkeletonBlock(width: 200, height: 18)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                ForEach(0..<4, id: \.self) { index in
                    HStack {
                        SkeletonBlock(width: 96, height: 14)
                        Spacer()
                        SkeletonBlock(width: 72, height: 14)
                    }
                    .padding(MonacoTheme.Space.m)
                    .overlay(alignment: .bottom) { if index < 3 { MonacoRule() } }
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.top, MonacoTheme.Space.m)
        .accessibilityHidden(true)
    }
}
