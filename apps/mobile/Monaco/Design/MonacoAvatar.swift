import MonacoCore
import SwiftUI

enum MonacoAvatarSize: CGFloat {
    case author = 24
    case chat = 26
    case comment = 30
    case feed = 32
    case preview = 34
    case rank = 36
    case activity = 38
    case row = 40
    case identity = 64
    case edit = 76
    case large = 80
}

enum MonacoAvatarRing {
    case surface, identity

    static let width: CGFloat = 2
    private static let identityOpacity: Double = 0.3

    var color: Color {
        switch self {
        case .surface: return MonacoTheme.surface
        case .identity: return MonacoTheme.onIdentity.opacity(Self.identityOpacity)
        }
    }
}

/// A member's photo, else their initials in white on a tint picked from their id (the name
/// when the screen has no id), so a renamed member keeps their colour.
struct MonacoAvatar: View {
    let photoURL: String?
    let displayName: String
    var size: CGFloat = 40
    var seed: String? = nil
    var ring: MonacoAvatarRing? = nil

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
        .monacoPrivate()
        .overlay {
            if let ring {
                Circle().strokeBorder(ring.color, lineWidth: MonacoAvatarRing.width)
            }
        }
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

extension MonacoAvatar {
    init(
        photoURL: String?, displayName: String, size: MonacoAvatarSize, seed: String? = nil,
        ring: MonacoAvatarRing? = nil
    ) {
        self.init(photoURL: photoURL, displayName: displayName, size: size.rawValue, seed: seed, ring: ring)
    }
}

struct MonacoFaceStack: View {
    struct Member: Identifiable {
        let id: String
        let displayName: String
        let photoURL: String?
    }

    let members: [Member]
    var total: Int? = nil

    private static let faceSize: CGFloat = 24
    private static let overlap: CGFloat = 6
    private static let maxFaces = 4
    private static let spokenNames = 3

    private var count: Int { max(total ?? members.count, members.count) }
    private var shown: [Member] { Array(members.prefix(Self.maxFaces)) }
    private var extra: Int { count - shown.count }

    static func spokenLabel(names: [String], total: Int) -> String {
        let named = names.prefix(spokenNames).joined(separator: ", ")
        let others = total - min(names.count, spokenNames)
        guard others > 0 else { return named }
        return "\(named) and \(others) others"
    }

    var body: some View {
        HStack(spacing: -Self.overlap) {
            ForEach(shown) { member in
                MonacoAvatar(
                    photoURL: member.photoURL, displayName: member.displayName, size: Self.faceSize,
                    seed: member.id, ring: .surface)
            }
            if extra > 0 {
                Text("+\(extra)")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
                    .minimumScaleFactor(0.5)
                    .frame(width: Self.faceSize, height: Self.faceSize)
                    .background(MonacoTheme.surfaceSunken, in: Circle())
                    .overlay { Circle().strokeBorder(MonacoTheme.surface, lineWidth: MonacoAvatarRing.width) }
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(names: members.map(\.displayName), total: count))
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
