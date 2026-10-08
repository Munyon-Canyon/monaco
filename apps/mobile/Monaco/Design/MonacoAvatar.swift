import MonacoCore
import SwiftUI

/// A member's photo, else their initials in white on a tint picked from their id (the name
/// when the screen has no id), so a renamed member keeps their colour.
struct MonacoAvatar: View {
    let photoURL: String?
    let displayName: String
    var size: CGFloat = 40
    var seed: String? = nil

    private var resolvedURL: URL? {
        let trimmed = photoURL?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !trimmed.isEmpty else { return nil }
        return URL(string: trimmed)
    }

    @State private var loadedImage: UIImage?
    @State private var didFail = false

    var body: some View {
        Group {
            if let resolvedURL {
                // A photo seen before is drawn on the first pass, with no placeholder flash.
                if let image = loadedImage ?? MonacoRemoteImageStore.avatars.cachedImage(for: resolvedURL) {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFill()
                } else if didFail {
                    placeholder
                } else {
                    placeholder
                }
            } else {
                placeholder
            }
        }
        .frame(width: size, height: size)
        .clipShape(Circle())
        .accessibilityHidden(true)
        .task(id: resolvedURL) {
            await loadPhoto()
        }
    }

    private func loadPhoto() async {
        loadedImage = nil
        didFail = false
        guard let resolvedURL else { return }
        let store = MonacoRemoteImageStore.avatars
        if let cached = store.cachedImage(for: resolvedURL) {
            loadedImage = cached
            return
        }
        let image = await store.image(for: resolvedURL)
        guard !Task.isCancelled else { return }
        withAnimation(.easeOut(duration: 0.2)) {
            loadedImage = image
            didFail = image == nil
        }
    }

    private var placeholder: some View {
        let key = (seed ?? "").isEmpty ? displayName : seed ?? ""
        return Circle()
            .fill(MonacoTheme.CabalTint.forGroupId(key).fill)
            .overlay {
                Text(CabalMark.initials(for: displayName))
                    .font(.system(size: size * 0.4, weight: .semibold))
                    .foregroundStyle(Color.white)
                    .lineLimit(1)
                    .minimumScaleFactor(0.5)
            }
    }
}

#Preview {
    HStack(spacing: 16) {
        MonacoAvatar(photoURL: nil, displayName: "Logan Norman", size: 96)
        MonacoAvatar(photoURL: nil, displayName: "", size: 44, seed: "user-2")
        MonacoAvatar(photoURL: nil, displayName: "Ana", size: 28)
    }
    .padding()
    .monacoCanvas()
}
