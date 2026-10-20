import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedShareContent {
    static func text(for item: Components.Schemas.FeedItem) -> String {
        let name = item.cabalName.map { " in \($0)" } ?? ""
        return "\(item.title)\(name) on Monaco"
    }

    static func url(links: InviteLinks?) -> URL {
        links?.shareURL ?? ShareCardContent.fallbackURL
    }
}

struct FeedShareButton<Label: View>: View {
    let item: Components.Schemas.FeedItem
    @ViewBuilder let label: () -> Label

    @Environment(AppEnvironment.self) private var environment
    @State private var items: [Any]?
    @State private var isPreparing = false

    var body: some View {
        Button {
            Task { await prepare() }
        } label: {
            label()
        }
        .buttonStyle(.plain)
        .disabled(isPreparing)
        .sheet(
            isPresented: Binding(get: { items != nil }, set: { if !$0 { items = nil } })
        ) {
            ActivitySheet(items: items ?? []) { items = nil }
                .presentationDetents([.fraction(MonacoSheetDetents.halfFraction)])
        }
    }

    private func prepare() async {
        isPreparing = true
        defer { isPreparing = false }
        let model = InviteLinkModel(api: environment.api, hints: environment.hints)
        await model.load()
        items = [FeedShareContent.text(for: item), FeedShareContent.url(links: model.links)]
    }
}
