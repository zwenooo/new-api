package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
)

type epayGatewayCheckout struct {
	GatewayTradeNo string
	PayPageURL     string
	QRCode         string
	QRImageURL     string
}

type epayGatewayPurchaseResponse struct {
	Code    interface{} `json:"code"`
	Msg     string      `json:"msg"`
	TradeNo string      `json:"trade_no"`
	OId     string      `json:"O_id"`
	PayURL  string      `json:"payurl"`
	QRCode  string      `json:"qrcode"`
	Image   string      `json:"img"`
}

func buildEpayEndpointURL(client *epay.Client, endpoint string) (*url.URL, error) {
	if client == nil {
		return nil, errors.New("当前管理员未配置支付信息")
	}
	if client.BaseUrl == nil {
		return nil, errors.New("支付地址无效")
	}

	target := *client.BaseUrl
	target.RawQuery = ""
	target.Fragment = ""
	basePath := strings.TrimSpace(target.Path)
	switch {
	case basePath == "" || basePath == "/":
		target.Path = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	case strings.HasSuffix(strings.ToLower(basePath), ".php"):
		dirPath := path.Dir(basePath)
		if dirPath == "." {
			dirPath = ""
		}
		target.Path = path.Join(dirPath, strings.TrimSpace(endpoint))
	default:
		target.Path = path.Join(basePath, strings.TrimSpace(endpoint))
	}
	return &target, nil
}

func resolveEpayClientIP(c *gin.Context) (string, error) {
	if c == nil {
		return "", errors.New("请求上下文无效")
	}
	clientIP := strings.TrimSpace(c.ClientIP())
	if clientIP == "" {
		return "", errors.New("未获取到客户端 IP，请检查反向代理配置")
	}
	if parsed := net.ParseIP(clientIP); parsed == nil {
		return "", fmt.Errorf("客户端 IP 无效: %s", clientIP)
	}
	return clientIP, nil
}

func createEpayGatewayCheckout(
	c *gin.Context,
	client *epay.Client,
	cfg PayOrderConfig,
	tradeNo string,
	amountFen int64,
	epayMethod string,
	notifyURL *url.URL,
	returnURL *url.URL,
) (*epayGatewayCheckout, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, errors.New("trade_no 不能为空")
	}
	if amountFen <= 0 {
		return nil, errors.New("money 必须大于0")
	}
	if notifyURL == nil || returnURL == nil {
		return nil, errors.New("支付回调地址无效")
	}

	clientIP, err := resolveEpayClientIP(c)
	if err != nil {
		return nil, err
	}
	targetURL, err := buildEpayEndpointURL(client, "mapi.php")
	if err != nil {
		return nil, err
	}

	formValues := epay.GenerateParams(map[string]string{
		"pid":          strings.TrimSpace(client.Config.PartnerID),
		"type":         strings.TrimSpace(epayMethod),
		"out_trade_no": strings.TrimSpace(tradeNo),
		"notify_url":   notifyURL.String(),
		"name":         strings.TrimSpace(cfg.EpayProductName()),
		"money":        formatFenYuan(amountFen),
		"clientip":     clientIP,
		"device":       string(epay.PC),
		"return_url":   returnURL.String(),
	}, strings.TrimSpace(client.Config.Key))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range formValues {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, targetURL.String(), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("支付网关下单失败: HTTP %d", resp.StatusCode)
	}

	var gatewayResp epayGatewayPurchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&gatewayResp); err != nil {
		return nil, err
	}
	if !isEpaySuccessCode(gatewayResp.Code) {
		if msg := strings.TrimSpace(gatewayResp.Msg); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, errors.New("支付网关下单失败")
	}

	checkout := &epayGatewayCheckout{
		GatewayTradeNo: strings.TrimSpace(gatewayResp.TradeNo),
		PayPageURL:     strings.TrimSpace(gatewayResp.PayURL),
		QRCode:         strings.TrimSpace(gatewayResp.QRCode),
		QRImageURL:     strings.TrimSpace(gatewayResp.Image),
	}
	if checkout.GatewayTradeNo == "" {
		checkout.GatewayTradeNo = strings.TrimSpace(gatewayResp.OId)
	}
	if checkout.QRCode == "" && checkout.QRImageURL == "" {
		return nil, errors.New("支付网关没有返回二维码")
	}
	return checkout, nil
}
