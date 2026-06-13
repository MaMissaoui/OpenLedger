package domain

// AccountType mirrors GnuCash's account-type strings. The stored value is the
// same token GnuCash uses, which keeps import/export a direct mapping.
type AccountType string

const (
	AccountRoot       AccountType = "ROOT"
	AccountAsset      AccountType = "ASSET"
	AccountBank       AccountType = "BANK"
	AccountCash       AccountType = "CASH"
	AccountCredit     AccountType = "CREDIT"
	AccountLiability  AccountType = "LIABILITY"
	AccountStock      AccountType = "STOCK"
	AccountMutual     AccountType = "MUTUAL"
	AccountCurrency   AccountType = "CURRENCY"
	AccountIncome     AccountType = "INCOME"
	AccountExpense    AccountType = "EXPENSE"
	AccountEquity     AccountType = "EQUITY"
	AccountReceivable AccountType = "RECEIVABLE"
	AccountPayable    AccountType = "PAYABLE"
	AccountTrading    AccountType = "TRADING"
)

// Account is a node in the chart-of-accounts tree, denominated in one
// commodity. It corresponds to a row in GnuCash's accounts table.
type Account struct {
	GUID          string
	Name          string
	Type          AccountType
	CommodityGUID string
	ParentGUID    string
	Code          string
	Description   string
	Placeholder   bool
	Hidden        bool
}

// IsDebitNormal reports whether the account type increases with a positive
// (debit) amount. This drives sign conventions in balances and reports:
// asset/expense-style accounts are debit-normal, the rest are credit-normal.
func (t AccountType) IsDebitNormal() bool {
	switch t {
	case AccountAsset, AccountBank, AccountCash, AccountStock, AccountMutual,
		AccountCurrency, AccountExpense, AccountReceivable, AccountTrading:
		return true
	default:
		return false
	}
}
