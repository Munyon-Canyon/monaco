import PhotosUI
import SwiftUI

/// The sheet behind the avatar: Monaco's 22 characters to pick from, and the photo library
/// for a member who would rather be themselves. Picking a character uploads it as the
/// profile photo, so every device and every other member sees the same face and the
/// profile needs no second field for it. The default is a hash of the member's id, and
/// two friends in a cabal land on the same character now and then; this is how one of
/// them stops being the other.
struct FacePickerSheet: View {
    /// The character the member wears today because they have no photo. Ringed in the grid;
    /// nil when a photo is set, since a stored photo says nothing about which character it was.
    let currentCharacter: MonacoCharacter?
    let onPickCharacter: (MonacoCharacter) -> Void
    let onPickPhoto: (PhotosPickerItem) -> Void

    @State private var selection: PhotosPickerItem?

    private static let columns = 4

    private let rows: [[MonacoCharacter]] = stride(from: 0, to: MonacoCharacter.allCases.count, by: columns).map {
        Array(MonacoCharacter.allCases[$0..<min($0 + columns, MonacoCharacter.allCases.count)])
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            Text("Pick a character, or use a photo.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.muted)

            PhotosPicker(selection: $selection, matching: .images, photoLibrary: .shared()) {
                Text("Choose a photo")
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("face-choose-photo")

            Grid(horizontalSpacing: MonacoTheme.Space.m, verticalSpacing: MonacoTheme.Space.m) {
                ForEach(rows, id: \.first) { row in
                    GridRow {
                        ForEach(row, id: \.self) { character in
                            face(character)
                        }
                    }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.xl)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: "Your face")
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("face-picker-sheet")
        .onChange(of: selection) { _, item in
            guard let item else { return }
            onPickPhoto(item)
        }
    }

    private func face(_ character: MonacoCharacter) -> some View {
        let isCurrent = character == currentCharacter
        return Button {
            onPickCharacter(character)
        } label: {
            Image(character.imageName)
                .resizable()
                .interpolation(.high)
                .scaledToFill()
                .frame(width: 64, height: 64)
                .clipShape(Circle())
                .overlay {
                    Circle().strokeBorder(
                        isCurrent ? MonacoTheme.accent : MonacoTheme.hairline,
                        lineWidth: isCurrent ? 3 : 1
                    )
                }
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(character.spokenName)
        .accessibilityAddTraits(isCurrent ? .isSelected : [])
        .accessibilityIdentifier("face-option-\(character.rawValue)")
    }
}

#Preview {
    Color.clear
        .sheet(isPresented: .constant(true)) {
            FacePickerSheet(currentCharacter: .bev, onPickCharacter: { _ in }, onPickPhoto: { _ in })
        }
}
