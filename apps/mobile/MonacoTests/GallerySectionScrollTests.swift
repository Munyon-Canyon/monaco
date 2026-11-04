#if DEBUG
import SwiftUI
import Testing

@testable import Monaco

struct GallerySectionScrollTests {
    private let flag = SampleScrollAnchor.flag

    @Test func aSectionIdIsReturnedAsIs() {
        #expect(SampleScrollAnchor.section(in: [flag, "fields"]) == "fields")
        #expect(SampleScrollAnchor.section(in: ["Monaco", flag, "buttons"]) == "buttons")
    }

    @Test func positionsAndMissingValuesAreNotSections() {
        #expect(SampleScrollAnchor.section(in: [flag, "center"]) == nil)
        #expect(SampleScrollAnchor.section(in: [flag, "bottom"]) == nil)
        #expect(SampleScrollAnchor.section(in: [flag, "0.5"]) == nil)
        #expect(SampleScrollAnchor.section(in: ["Monaco", flag]) == nil)
        #expect(SampleScrollAnchor.section(in: ["Monaco"]) == nil)
    }

    @Test func titlesBecomeSlugs() {
        #expect(GalleryGroup<EmptyView>.slug("Small, links and tiles") == "small-links-and-tiles")
        #expect(GalleryGroup<EmptyView>.slug("Fields") == "fields")
        #expect(GalleryGroup<EmptyView>.slug("Buttons") == "buttons")
        #expect(GalleryGroup<EmptyView>.slug("  --Two  words-- ") == "two-words")
    }
}
#endif
