package domain

// Book is the top-level container for one set of books (one ledger/company). It
// owns a root account whose descendants form the chart of accounts, and a
// separate root for scheduled-transaction templates. It corresponds to a row in
// GnuCash's books table.
type Book struct {
	GUID             string
	RootAccountGUID  string
	RootTemplateGUID string
}
