package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	merchantCode = "MX6072"
	payItemID    = "9405967"
	verifyURL    = "https://qa.interswitchng.com/collections/api/v1/gettransaction.json"
	tokenTTL     = 24 * time.Hour
)

type user struct {
	firstName string
	lastName  string
	email     string
	password  string
	role      string
}

type tokenEntry struct {
	email     string
	role      string
	expiresAt time.Time
}

type authStore struct {
	mu     sync.RWMutex
	users  map[string]user
	tokens map[string]tokenEntry
}

var store = &authStore{
	users:  make(map[string]user),
	tokens: make(map[string]tokenEntry),
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *authStore) register(firstName, lastName, email, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[email]; exists {
		return fmt.Errorf("email already registered")
	}
	s.users[email] = user{
		firstName: firstName,
		lastName:  lastName,
		email:     email,
		password:  password,
		role:      "buyer",
	}
	return nil
}

func (s *authStore) login(email, password string) (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[email]
	if !ok || u.password != password {
		return "", "", false
	}
	token := generateToken()
	s.tokens[token] = tokenEntry{
		email:     email,
		role:      u.role,
		expiresAt: time.Now().Add(tokenTTL),
	}
	return token, u.role, true
}

func (s *authStore) validate(token string) (tokenEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.tokens[token]
	if !ok || time.Now().After(entry.expiresAt) {
		return tokenEntry{}, false
	}
	return entry, true
}

func (s *authStore) upgradToSeller(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.tokens[token]
	if !ok || time.Now().After(entry.expiresAt) {
		return fmt.Errorf("invalid or expired token")
	}

	u, ok := s.users[entry.email]
	if !ok {
		return fmt.Errorf("user not found")
	}

	if u.role == "seller" {
		return fmt.Errorf("already a seller")
	}

	u.role = "seller"
	s.users[entry.email] = u

	entry.role = "seller"
	s.tokens[token] = entry

	return nil
}

func (s *authStore) logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

type RegisterRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.FirstName == "" || req.LastName == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "all fields are required", http.StatusBadRequest)
		return
	}
	if err := store.register(req.FirstName, req.LastName, req.Email, req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "registered successfully"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	token, role, ok := store.login(req.Email, req.Password)
	if !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"role":       role,
		"expires_in": tokenTTL.String(),
	})
}

func handleUpgradeToSeller(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	if token == "" {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}
	if err := store.upgradToSeller(token); err != nil {
		if err.Error() == "already a seller" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "account upgraded to seller"})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	store.logout(token)
	w.WriteHeader(http.StatusNoContent)
}

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

	mux.HandleFunc("/auth/register", handleRegister)
	mux.HandleFunc("/auth/login", handleLogin)
	mux.HandleFunc("/auth/logout", handleLogout)
	mux.HandleFunc("/account/upgrade", handleUpgradeToSeller)

	mux.HandleFunc("/checkout", handleCheckout)
	mux.HandleFunc("/payment/verify", handleVerify)

	fmt.Println("server running on :8080")
	http.ListenAndServe(":8080", mux)
}
