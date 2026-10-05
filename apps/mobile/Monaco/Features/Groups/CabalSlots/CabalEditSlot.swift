import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalEditSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalEditRow(cabalID: context.cabalID)
    }
}

private struct CabalEditRow: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: CabalEditModel?

    var body: some View {
        CabalEditEntry(model: model, pictureWriter: { LiveCabalPictureWriter(api: environment.api) })
            .task {
                let model = preparedModel()
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { visible in
                model?.setVisible(visible)
            }
    }

    private func preparedModel() -> CabalEditModel {
        if let model { return model }
        let created = CabalEditModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct CabalEditEntry: View {
    let model: CabalEditModel?
    let pictureWriter: () -> any CabalPictureWriting

    var body: some View {
        if let model, model.isCreator, let cabal = model.cabal {
            NavigationLink {
                EditCabalView(model: model, cabal: cabal, pictureWriter: pictureWriter())
            } label: {
                HStack {
                    Text(EditCabalCopy.rowTitle)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                    Spacer()
                    Image(systemName: "chevron.right")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityHidden(true)
                }
                .padding(MonacoTheme.Space.m)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("cabal-edit-row")
        } else {
            Color.clear.frame(height: 0)
        }
    }
}

#if DEBUG
final class CabalEditSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalEditHarness") else { return nil }
        let role = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "creator"
        return AnyView(CabalEditHarnessScreen(role: role))
    }
}

private struct CabalEditHarnessScreen: View {
    let role: String
    @State private var model: CabalEditModel?

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                CabalEditEntry(model: model, pictureWriter: { SampleCabalPictureWriter() })
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .navigationTitle("Details")
            .task {
                let created = model ?? CabalEditModel.preview(.sample(role: role))
                model = created
                await created.load()
            }
        }
    }
}
#endif
