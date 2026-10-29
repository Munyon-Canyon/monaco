import MonacoCore
import SwiftUI

struct MarketsChips: View {
    let selected: MonacoCore.StocksTabModel.Browse
    let select: (MonacoCore.StocksTabModel.Browse) -> Void

    var body: some View {
        MonacoChipBar(
            items: MonacoCore.StocksTabModel.availableBrowses, selected: selected, title: \.title,
            identifierPrefix: "markets-chip", select: select)
    }
}
