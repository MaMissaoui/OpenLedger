package domain

import "time"

// Price is an exchange rate / quote: one unit of Commodity is worth Value units
// of Currency on Date. It corresponds to a row in GnuCash's prices table.
// Source and Type are GnuCash's free-form provenance fields (e.g. "user:price").
type Price struct {
	GUID          string
	CommodityGUID string
	CurrencyGUID  string
	Date          time.Time
	Source        string
	Type          string
	Value         GncNumeric
}
