import Foundation
import Testing

struct AppearanceLockTests {
    @Test func appStaysDarkWhateverThePhoneSetting() {
        let style = Bundle.main.object(forInfoDictionaryKey: "UIUserInterfaceStyle") as? String

        #expect(style == "Dark")
    }
}
