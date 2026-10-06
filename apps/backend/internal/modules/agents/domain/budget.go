package domain

import "github.com/monaco/monaco/apps/backend/internal/platform/money"

func AvailableBudget(
	allocation, executedBuys, inFlightBuys, confirmedSellProceeds money.Micros,
) (money.Micros, error) {
	credit, err := allocation.Add(confirmedSellProceeds)
	if err != nil {
		return money.Micros{}, err
	}
	spent, err := executedBuys.Add(inFlightBuys)
	if err != nil {
		return money.Micros{}, err
	}
	if credit.Cmp(spent) <= 0 {
		return money.Micros{}, nil
	}
	return credit.Sub(spent)
}

func Sellable(bought, sold, inFlightSells, treasuryHeld money.BaseUnits) (money.BaseUnits, error) {
	spent, err := sold.Add(inFlightSells)
	if err != nil {
		return money.BaseUnits{}, err
	}
	unsold, err := clampedSub(bought, spent)
	if err != nil {
		return money.BaseUnits{}, err
	}
	order, err := unsold.Cmp(treasuryHeld)
	if err != nil {
		return money.BaseUnits{}, err
	}
	if order > 0 {
		return treasuryHeld, nil
	}
	return unsold, nil
}

func clampedSub(a, b money.BaseUnits) (money.BaseUnits, error) {
	order, err := a.Cmp(b)
	if err != nil {
		return money.BaseUnits{}, err
	}
	if order <= 0 {
		return money.NewBaseUnits(0, a.Decimals()), nil
	}
	return a.Sub(b)
}
