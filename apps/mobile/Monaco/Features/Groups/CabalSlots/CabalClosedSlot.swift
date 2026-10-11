import MonacoCore
import SwiftUI

enum CabalClosedSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalClosedLive(cabalID: context.cabalID)
    }
}

private struct CabalClosedLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalClosedModel?

    var body: some View {
        CabalClosedRow(isClosed: model?.isClosed == true)
            .task(id: retry.tick) {
                let model =
                    self.model ?? CabalClosedModel(api: environment.api, hints: environment.hints, cabalID: cabalID)
                self.model = model
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }
}

struct CabalClosedRow: View {
    let isClosed: Bool

    var body: some View {
        if isClosed {
            VStack(spacing: 0) {
                MonacoRule()
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                    Image(systemName: "exclamationmark.octagon")
                        .font(.body)
                        .foregroundStyle(MonacoTheme.loss)
                        .accessibilityHidden(true)
                    Text(CabalClosedModel.message)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(minHeight: MonacoRowLayout.minHeight)
                MonacoRule()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("cabal-closed-row")
        } else {
            Color.clear.frame(height: 0)
        }
    }
}

#if DEBUG
final class CabalClosedSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalClosedHarness") else { return nil }
        let state = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "banned"
        return AnyView(
            VStack(spacing: 0) {
                CabalClosedRow(isClosed: state == "banned")
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
        )
    }
}
#endif
