package ledger

import (
	"fmt"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

// AccountType is the accounting class of an account.
type AccountType string

const (
	TypeAsset     AccountType = "asset"
	TypeExpense   AccountType = "expense"
	TypeLiability AccountType = "liability"
	TypeEquity    AccountType = "equity"
	TypeRevenue   AccountType = "revenue"
)

// ParseAccountType returns s as an AccountType, or ErrInvalid when s is not one of the five types.
func ParseAccountType(s string) (AccountType, error) {
	switch t := AccountType(s); t {
	case TypeAsset, TypeExpense, TypeLiability, TypeEquity, TypeRevenue:
		return t, nil
	}
	return "", fmt.Errorf("%w: account type %q must be one of asset, expense, liability, equity, revenue", ErrInvalid, s)
}

// debitNormal reports whether t's balance grows with debits; the other types grow with credits.
func debitNormal(t AccountType) bool {
	return t == TypeAsset || t == TypeExpense
}

// tbCode returns the TigerBeetle account code that identifies t, or 0 for an unknown type.
func tbCode(t AccountType) uint16 {
	switch t {
	case TypeAsset:
		return 1
	case TypeExpense:
		return 2
	case TypeLiability:
		return 3
	case TypeEquity:
		return 4
	case TypeRevenue:
		return 5
	}
	return 0
}

// tbFlags returns the TigerBeetle account flags for t. They always keep the balance after every transfer,
// and unless allowNegative is set they make TigerBeetle reject any transfer that would push the balance
// below zero on t's normal side.
func tbFlags(t AccountType, allowNegative bool) uint16 {
	f := tb.AccountFlags{History: true}
	switch {
	case allowNegative:
	case debitNormal(t):
		f.CreditsMustNotExceedDebits = true
	default:
		f.DebitsMustNotExceedCredits = true
	}
	return f.ToUint16()
}
