import Foundation
import MonacoCore

enum LedgerShareEyes {
    static let key = "ledger.share.eyes"

    static func saved(_ defaults: UserDefaults = .standard) -> ShareEyes? {
        guard let flags = defaults.array(forKey: key) as? [Bool], flags.count == 3 else { return nil }
        return ShareEyes(pnlDollars: flags[0], buySell: flags[1], code: flags[2])
    }

    static func save(_ eyes: ShareEyes, to defaults: UserDefaults = .standard) {
        defaults.set([eyes.pnlDollars, eyes.buySell, eyes.code], forKey: key)
    }
}
