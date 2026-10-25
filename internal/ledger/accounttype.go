package ledger

import "fmt"

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
