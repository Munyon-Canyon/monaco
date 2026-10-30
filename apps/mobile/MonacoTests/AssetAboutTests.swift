import MonacoCore
import Testing

@testable import Monaco

struct AssetAboutTests {
    @Test func theCompanyProfileStaysHiddenUntilItIsBuilt() {
        #expect(LaunchFeatures.companyProfile == false)
        #expect(AssetAbout.isShown(kind: .stock) == false)
        #expect(AssetAbout.isShown(kind: .stock, hasProfile: true))
    }

    @Test func thePreIpoDisclosureShowsWhateverTheProfileFlagSays() {
        #expect(AssetAbout.isShown(kind: .preIpo))
        #expect(AssetAbout.isShown(kind: .preIpo, hasProfile: false))
    }
}
