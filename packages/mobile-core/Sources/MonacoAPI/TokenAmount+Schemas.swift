import Foundation

extension Components.Schemas.Proposal {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
    public var quoteOutAmountValue: Decimal { TokenAmount.decimal(quoteOutAmountStr, fallback: quoteOutAmount) ?? 0 }
}

extension Components.Schemas.ProposalStats {
    public var sharesAtomicsValue: Decimal? { TokenAmount.decimal(sharesAtomicsStr, fallback: sharesAtomics) }
}

extension Components.Schemas.ProposalDetail {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
    public var quoteOutAmountValue: Decimal { TokenAmount.decimal(quoteOutAmountStr, fallback: quoteOutAmount) ?? 0 }
}

extension Components.Schemas.ProposalDetail.SwapPayload {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
}

extension Components.Schemas.PendingVote {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
}

extension Components.Schemas.ProposalPriceComparison {
    public var quoteOutAmountValue: Decimal { TokenAmount.decimal(quoteOutAmountStr, fallback: quoteOutAmount) ?? 0 }
}

extension Components.Schemas.TradePreview {
    public var quoteOutAmountValue: Decimal? { TokenAmount.decimal(quoteOutAmountStr, fallback: quoteOutAmount) }
}

extension Components.Schemas.SwapDetail {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
}

extension Components.Schemas.CabalActivity {
    public var unitsValue: Decimal? { TokenAmount.decimal(unitsStr, fallback: units) }
}

extension Components.Schemas.CabalHolding {
    public var tokenAmountValue: Decimal { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) ?? 0 }
}

extension Components.Schemas.FeedItem {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
    public var filledTokenAmountValue: Decimal? {
        TokenAmount.decimal(filledTokenAmountStr, fallback: filledTokenAmount)
    }
}

extension Components.Schemas.CatchUpProposal {
    public var tokenAmountValue: Decimal? { TokenAmount.decimal(tokenAmountStr, fallback: tokenAmount) }
}
