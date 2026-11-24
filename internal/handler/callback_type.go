package handler

const (
	CallbackBuy           = "buy"
	CallbackSell          = "sell"
	CallbackStart         = "start"
	CallbackConnect       = "connect"
	CallbackPayment       = "payment"
	CallbackTrial         = "trial"
	CallbackActivateTrial = "activate_trial"
	CallbackReferral      = "referral"
	CallbackMobilePay     = "mobilepay"
	CallbackAdmin         = "admin"

	// CallbackCancelPendingPurchase releases a purchase that is still waiting for
	// a receipt screenshot, from inside the chat (U2). Callback data carries the
	// purchase id; the same callback with confirm=1 performs the cancellation.
	CallbackCancelPendingPurchase = "cancel_pending_purchase"
)
