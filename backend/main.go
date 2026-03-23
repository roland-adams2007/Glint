package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	merchantCode = "MX6072"
	payItemID    = "9405967"
	verifyURL    = "https://qa.interswitchng.com/collections/api/v1/gettransaction.json"
)

type CheckoutRequest struct {
	Amount      int    `json:"amount"`
	CustName    string `json:"cust_name"`
	CustEmail   string `json:"cust_email"`
	PayItemName string `json:"pay_item_name"`
	RedirectURL string `json:"site_redirect_url"`
}

type CheckoutResponse struct {
	MerchantCode    string `json:"merchant_code"`
	PayItemID       string `json:"pay_item_id"`
	TxnRef          string `json:"txn_ref"`
	Amount          int    `json:"amount"`
	Currency        int    `json:"currency"`
	CustName        string `json:"cust_name"`
	CustEmail       string `json:"cust_email"`
	PayItemName     string `json:"pay_item_name"`
	SiteRedirectURL string `json:"site_redirect_url"`
	Mode            string `json:"mode"`
}

type VerifyResponse struct {
	Amount                   int    `json:"Amount"`
	MerchantReference        string `json:"MerchantReference"`
	PaymentReference         string `json:"PaymentReference"`
	RetrievalReferenceNumber string `json:"RetrievalReferenceNumber"`
	TransactionDate          string `json:"TransactionDate"`
	ResponseCode             string `json:"ResponseCode"`
	ResponseDescription      string `json:"ResponseDescription"`
}

func generateTxnRef() string {
	return fmt.Sprintf("txn_%d", time.Now().UnixMilli())
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, "amount is required", http.StatusBadRequest)
		return
	}

	resp := CheckoutResponse{
		MerchantCode:    merchantCode,
		PayItemID:       payItemID,
		TxnRef:          generateTxnRef(),
		Amount:          req.Amount,
		Currency:        566,
		CustName:        req.CustName,
		CustEmail:       req.CustEmail,
		PayItemName:     req.PayItemName,
		SiteRedirectURL: req.RedirectURL,
		Mode:            "TEST",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	txnRef := r.URL.Query().Get("txn_ref")
	amount := r.URL.Query().Get("amount")

	if txnRef == "" || amount == "" {
		http.Error(w, "txn_ref and amount are required", http.StatusBadRequest)
		return
	}

	url := fmt.Sprintf("%s?merchantcode=%s&transactionreference=%s&amount=%s",
		verifyURL, merchantCode, txnRef, amount)

	res, err := http.Get(url)
	if err != nil {
		http.Error(w, "failed to reach interswitch", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		http.Error(w, "failed to read response", http.StatusInternalServerError)
		return
	}

	var verifyResp VerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		http.Error(w, "failed to parse response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(verifyResp)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", handleCheckout)
	mux.HandleFunc("/payment/verify", handleVerify)

	fmt.Println("server running on :8080")
	http.ListenAndServe(":8080", mux)
}
