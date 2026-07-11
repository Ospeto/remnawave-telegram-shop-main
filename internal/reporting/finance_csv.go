package reporting

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

func FormatFinanceReportCSV(report FinanceReport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"section", "key", "value"}); err != nil {
		return nil, err
	}
	rows := [][]string{
		{"meta", "period", report.Period},
		{"meta", "timezone", report.Timezone},
		{"meta", "currency", report.Currency},
		{"meta", "range_start", report.RangeStart},
		{"meta", "range_end", report.RangeEnd},
		{"meta", "in_progress", fmt.Sprintf("%t", report.InProgress)},
		{"current", "gross_service_revenue", fmt.Sprintf("%.2f", RoundMoney(report.Current.GrossServiceRevenue))},
		{"current", "refunds", fmt.Sprintf("%.2f", RoundMoney(report.Current.Refunds))},
		{"current", "net_service_revenue", fmt.Sprintf("%.2f", RoundMoney(report.Current.NetServiceRevenue))},
		{"current", "cash_collected", fmt.Sprintf("%.2f", RoundMoney(report.Current.CashCollected))},
		{"current", "wallet_topups", fmt.Sprintf("%.2f", RoundMoney(report.Current.WalletTopUps))},
		{"current", "wallet_spend", fmt.Sprintf("%.2f", RoundMoney(report.Current.WalletSpend))},
		{"current", "successful_orders", fmt.Sprintf("%d", report.Current.SuccessfulOrders)},
		{"current", "unique_customers", fmt.Sprintf("%d", report.Current.UniqueCustomers)},
		{"current", "average_order_value", fmt.Sprintf("%.2f", RoundMoney(report.Current.AverageOrderValue))},
		{"current", "new_subscriptions", fmt.Sprintf("%d", report.Current.NewSubscriptions)},
		{"current", "extensions", fmt.Sprintf("%d", report.Current.Extensions)},
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	for _, c := range report.Categories {
		if err := w.Write([]string{"category", c.Category, fmt.Sprintf("%d:%.2f", c.Orders, RoundMoney(c.Amount))}); err != nil {
			return nil, err
		}
	}
	for _, m := range report.Methods {
		if err := w.Write([]string{"method", m.Method, fmt.Sprintf("%d:%.2f:%.2f", m.Transactions, RoundMoney(m.ServiceRevenue), RoundMoney(m.CashCollected))}); err != nil {
			return nil, err
		}
	}
	for _, tr := range report.Trend {
		if err := w.Write([]string{"trend", tr.PeriodStart, fmt.Sprintf("%.2f:%.2f:%.2f", RoundMoney(tr.Metrics.GrossServiceRevenue), RoundMoney(tr.Metrics.Refunds), RoundMoney(tr.Metrics.NetServiceRevenue))}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
