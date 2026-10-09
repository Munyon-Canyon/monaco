import PhotosUI
import SwiftUI

/// The sheet behind the avatar: the eight pixel animals to pick from, and the photo library
/// for a member who would rather be themselves. Picking an animal uploads it as the
/// profile photo, so every device and every other member sees the same face and the
/// profile needs no second field for it. The default is a hash of the member's id, and
/// two friends in a cabal of three land on the same animal about a third of the time;
/// this is how one of them stops being the other.
struct FacePickerSheet: View {
    /// The animal the member wears today because they have no photo. Ringed in the grid;
    /// nil when a photo is set, since a stored photo says nothing about which animal it was.
    let currentAnimal: PixelAnimal?
    let onPickAnimal: (PixelAnimal) -> Void
    let onPickPhoto: (PhotosPickerItem) -> Void

    @State private var selection: PhotosPickerItem?
    /// The sheet is as tall as the grid and the button. A medium detent left a blank third.
    @State private var contentHeight: CGFloat = 0

    @ScaledMetric(relativeTo: .title) private var titleAllowance: CGFloat = 64

    private let columns = Array(
        repeating: GridItem(.flexible(), spacing: MonacoTheme.Space.m),
        count: 4
    )

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                Text("Pick an animal, or use a photo.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)

                LazyVGrid(columns: columns, spacing: MonacoTheme.Space.m) {
                    ForEach(PixelAnimal.allCases, id: \.self) { animal in
                        face(animal)
                    }
                }

                PhotosPicker(selection: $selection, matching: .images, photoLibrary: .shared()) {
                    Text("Choose a photo")
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("face-choose-photo")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
            .onGeometryChange(for: CGFloat.self) {
                $0.size.height
            } action: {
                contentHeight = $0
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoSheet(title: "Your face")
        .presentationDetents(contentHeight > 0 ? [.height(contentHeight + titleAllowance)] : [.medium])
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("face-picker-sheet")
        .onChange(of: selection) { _, item in
            guard let item else { return }
            onPickPhoto(item)
        }
    }

    private func face(_ animal: PixelAnimal) -> some View {
        let isCurrent = animal == currentAnimal
        return Button {
            onPickAnimal(animal)
        } label: {
            Image(animal.imageName)
                .resizable()
                .interpolation(.none)
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
        .accessibilityLabel(animal.spokenName)
        .accessibilityAddTraits(isCurrent ? .isSelected : [])
        .accessibilityIdentifier("face-option-\(animal.rawValue)")
    }
}

#Preview {
    Color.clear
        .sheet(isPresented: .constant(true)) {
            FacePickerSheet(currentAnimal: .cat, onPickAnimal: { _ in }, onPickPhoto: { _ in })
        }
}
