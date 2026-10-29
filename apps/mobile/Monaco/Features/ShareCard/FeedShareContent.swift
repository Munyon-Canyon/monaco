import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedShareContent {
    static func text(for item: Components.Schemas.FeedItem) -> String {
        let name = item.cabalName.map { " in \($0)" } ?? ""
        return "\(item.title)\(name) on Monaco"
    }

    static func url(links: InviteLinks?) -> URL {
        links?.shareURL ?? SharePayload.fallbackURL
    }

    static func payload(for item: Components.Schemas.FeedItem, links: InviteLinks?) -> SharePayload {
        SharePayload(title: text(for: item), url: url(links: links))
    }
}

struct FeedShareButton<Label: View>: View {
    let item: Components.Schemas.FeedItem
    @ViewBuilder let label: () -> Label

    @Environment(AppEnvironment.self) private var environment
    @State private var isPreparing = false

    var body: some View {
        Button {
            Task { await share() }
        } label: {
            label()
                .opacity(isPreparing ? 0 : 1)
                .overlay { if isPreparing { ProgressView() } }
        }
        .buttonStyle(.plain)
        .disabled(isPreparing)
    }

    private func share() async {
        isPreparing = true
        defer { isPreparing = false }
        let model = InviteLinkModel(api: environment.api, hints: environment.hints)
        await model.load()
        SystemShare.present(FeedShareContent.payload(for: item, links: model.links))
    }
}
