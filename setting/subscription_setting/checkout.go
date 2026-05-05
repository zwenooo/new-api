package subscription_setting

import "one-api/setting/config"

type SubscriptionCheckoutSettings struct {
	CheckoutMode   string `json:"checkout_mode"`
	TrafficMessage string `json:"traffic_message"`
	TrafficQRCode  string `json:"traffic_qrcode"`
	StoreNotice    string `json:"store_notice"`
}

var defaultSubscriptionCheckoutSettings = SubscriptionCheckoutSettings{
	CheckoutMode:   "payment",
	TrafficMessage: "",
	TrafficQRCode:  "",
	StoreNotice:    "",
}

var subscriptionCheckoutSettings = defaultSubscriptionCheckoutSettings

func init() {
	config.GlobalConfig.Register("subscription", &subscriptionCheckoutSettings)
}

func GetSubscriptionCheckoutSettings() *SubscriptionCheckoutSettings {
	return &subscriptionCheckoutSettings
}
