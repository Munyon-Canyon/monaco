#if DEBUG
import SwiftUI

struct GalleryAvatarSamples: View {
    private let sizes: [(name: String, size: MonacoAvatarSize)] = [
        ("author 24", .author), ("chat 26", .chat), ("comment 30", .comment), ("feed 32", .feed),
        ("preview 34", .preview), ("rank 36", .rank), ("activity 38", .activity), ("row 40", .row),
        ("identity 64", .identity), ("edit 76", .edit), ("large 80", .large),
    ]

    private let people = ["Maya Okafor", "Jon Reyes", "Priya Nair", "Sam Idowu", "Lena Fischer", "Tom Baker"]

    var body: some View {
        GalleryGroup(title: "Avatars") {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                GalleryCase(caption: "Default animals") {
                    GalleryGrid(minimum: 48) {
                        ForEach(PixelAnimal.allCases, id: \.self) { animal in
                            MonacoAvatar(photoURL: nil, displayName: animal.spokenName, size: 40, seed: animal.rawValue)
                        }
                    }
                }
                GalleryCase(caption: "Every size, without and with a ring") {
                    GalleryGrid(minimum: 130) {
                        ForEach(sizes, id: \.name) { entry in
                            GalleryCase(caption: entry.name) {
                                HStack(spacing: MonacoTheme.Space.s) {
                                    MonacoAvatar(photoURL: nil, displayName: "Maya Okafor", size: entry.size)
                                    MonacoAvatar(
                                        photoURL: nil, displayName: "Maya Okafor", size: entry.size, ring: .surface)
                                }
                            }
                        }
                    }
                }
                faces
            }
        }
    }

    private var faces: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            GalleryCase(caption: "Face stack, 3") {
                MonacoFaceStack(members: members(3))
            }
            GalleryCase(caption: "Face stack, 6 shows +2") {
                MonacoFaceStack(members: members(6))
            }
        }
    }

    private func members(_ count: Int) -> [MonacoFaceStack.Member] {
        people.prefix(count).enumerated().map {
            MonacoFaceStack.Member(id: "gallery-\($0.offset)", displayName: $0.element, photoURL: nil)
        }
    }
}

struct GalleryIdentitySamples: View {
    var body: some View {
        GalleryGroup(title: "Identity photo") {
            GalleryCase(caption: "Photo ring and pencil badge on the identity card") {
                ZStack(alignment: .bottomTrailing) {
                    MonacoAvatar(photoURL: nil, displayName: "Maya Okafor", size: .identity, ring: .identity)
                    PencilBadgeMark()
                        .offset(x: MonacoTheme.Space.xs, y: MonacoTheme.Space.xs)
                }
                .padding(MonacoTheme.Space.l)
                .background(
                    MonacoTheme.identityFill,
                    in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.identity, style: .continuous))
            }
        }
    }
}
#endif
