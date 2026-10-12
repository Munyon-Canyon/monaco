import MonacoAPI
import MonacoCore
import SwiftUI

enum ShareCardCopy {
    static let title = "Share"
    static let share = "Share"
    static let copyLink = "Copy link"
    static let copied = "Link copied."
    static let thing = "the card"
}

enum ShareCardAccess {
    static func canShareCabal(isMember: Bool, isActive: Bool, potLoaded: Bool) -> Bool {
        isMember && isActive && potLoaded
    }

    static func canShareMe(hasProfile: Bool, portfolioLoaded: Bool) -> Bool {
        hasProfile && portfolioLoaded
    }
}

struct ShareCardHost: View {
    @State private var model: ShareCardModel
    let load: (ShareCardModel) async -> Void

    init(api: APIClient, load: @escaping (ShareCardModel) async -> Void) {
        _model = State(initialValue: ShareCardModel(api: api))
        self.load = load
    }

    var body: some View {
        ShareCardSheet(model: model) { await load(model) }
    }
}

struct ShareCardSheet: View {
    let model: ShareCardModel
    let load: () async -> Void

    @Environment(\.dismiss) private var dismiss
    @Environment(ToastCenter.self) private var toasts: ToastCenter?
    @State private var image: UIImage?
    @State private var showsActivity = false

    var body: some View {
        NavigationStack {
            content
                .monacoSheet(title: ShareCardCopy.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("Done") { dismiss() }
                            .accessibilityIdentifier("share-card-done")
                    }
                }
        }
        .presentationDetents([.large])
        .task { if model.content == nil { await load() } }
    }

    @ViewBuilder private var content: some View {
        switch model.state {
        case .idle, .loading:
            SkeletonBlock(width: nil, height: 375, radius: MonacoTheme.Radius.card)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("share-card-loading")
        case .failed:
            MonacoErrorRow(thing: ShareCardCopy.thing, identifier: "share-card-failed") {
                Task { await load() }
            }
        case .loaded(let content):
            loaded(content)
        }
    }

    private func loaded(_ content: ShareCardContent) -> some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.m) {
                card(content)
                VStack(spacing: MonacoTheme.Space.s) {
                    Button {
                        showsActivity = true
                    } label: {
                        Label(ShareCardCopy.share, systemImage: "square.and.arrow.up")
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(image == nil)
                    .accessibilityIdentifier("share-card-share")
                    Button {
                        copy(content)
                    } label: {
                        Label(ShareCardCopy.copyLink, systemImage: "doc.on.doc")
                    }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("share-card-copy")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .task(id: content) {
            let face = await ShareCardRenderer.face(for: content.faceURL)
            image = ShareCardRenderer.render(content, face: face)
        }
        .sheet(isPresented: $showsActivity) {
            if let image {
                ActivitySheet(items: [image, content.shareURL]) { showsActivity = false }
                    .presentationDetents([.medium, .large])
            }
        }
    }

    @ViewBuilder private func card(_ content: ShareCardContent) -> some View {
        if let image {
            Image(uiImage: image)
                .resizable()
                .aspectRatio(ShareCardView.size.width / ShareCardView.size.height, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(content.spoken)
                .accessibilityIdentifier("share-card-image")
        } else {
            SkeletonBlock(width: nil, height: 375, radius: MonacoTheme.Radius.card)
                .accessibilityIdentifier("share-card-rendering")
        }
    }

    private func copy(_ content: ShareCardContent) {
        UIPasteboard.general.url = content.shareURL
        Haptics.selection()
        toasts?.current = MonacoToast(message: ShareCardCopy.copied, isSuccess: true)
    }
}
