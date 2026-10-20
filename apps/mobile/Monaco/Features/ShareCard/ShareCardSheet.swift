import MonacoAPI
import MonacoCore
import SwiftUI

enum ShareCardCopy {
    static let title = "Share"
    static let share = "Share"
    static let copyLink = "Copy link"
    static let copied = "Link copied."
    static let thing = "the card"
    static let previewHeight: CGFloat = 220
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

    @Environment(ToastCenter.self) private var toasts: ToastCenter?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var image: UIImage?
    @State private var showsActivity = false

    var body: some View {
        content
            .monacoSheet(
                title: ShareCardCopy.title, titleIdentifier: "share-card-title", detents: .votes, showsClose: true,
                closeIdentifier: "share-card-done"
            )
            .task { if model.content == nil { await load() } }
    }

    @ViewBuilder private var content: some View {
        switch model.state {
        case .idle, .loading:
            SkeletonBlock(width: nil, height: ShareCardCopy.previewHeight, radius: MonacoTheme.Radius.card)
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
            layout(content)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.m)
        }
        .scrollBounceBehavior(.basedOnSize)
        .task(id: content) {
            let face = await ShareCardRenderer.face(for: content.faceURL)
            image = ShareCardRenderer.render(content, face: face)
        }
        .sheet(isPresented: $showsActivity) {
            if let image {
                ActivitySheet(items: [image, content.shareURL]) { showsActivity = false }
                    .presentationDetents([.fraction(MonacoSheetDetents.halfFraction)])
            }
        }
    }

    @ViewBuilder private func layout(_ content: ShareCardContent) -> some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.m) {
                card(content)
                summary(content)
                actions(content)
            }
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.m) {
                card(content)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    summary(content)
                    Spacer(minLength: 0)
                    actions(content)
                }
                .frame(maxWidth: .infinity, minHeight: ShareCardCopy.previewHeight, alignment: .topLeading)
            }
        }
    }

    private func summary(_ content: ShareCardContent) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(content.title)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(2)
            if !content.subtitle.isEmpty {
                Text(content.subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("share-card-summary")
    }

    private func actions(_ content: ShareCardContent) -> some View {
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

    @ViewBuilder private func card(_ content: ShareCardContent) -> some View {
        if let image {
            Image(uiImage: image)
                .resizable()
                .aspectRatio(ShareCardView.size.width / ShareCardView.size.height, contentMode: .fit)
                .frame(height: ShareCardCopy.previewHeight)
                .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(content.spoken)
                .accessibilityIdentifier("share-card-image")
        } else {
            SkeletonBlock(
                width: ShareCardCopy.previewHeight * ShareCardView.size.width / ShareCardView.size.height,
                height: ShareCardCopy.previewHeight, radius: MonacoTheme.Radius.card
            )
            .accessibilityIdentifier("share-card-rendering")
        }
    }

    private func copy(_ content: ShareCardContent) {
        UIPasteboard.general.url = content.shareURL
        Haptics.selection()
        toasts?.current = MonacoToast(message: ShareCardCopy.copied, isSuccess: true)
    }
}
