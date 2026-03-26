
# Interswitch Payment - Frontend Integration Guide

The backend exposes two endpoints. The frontend owns the payment UI.

---

## How it works

1. Frontend calls `POST /checkout` and gets payment params back
2. Frontend passes params to Interswitch's JS widget and popup opens
3. User completes payment inside the popup
4. `onComplete` fires with the result
5. Frontend calls `GET /payment/verify` and backend confirms with Interswitch
6. If verified, give value

---

## Auth

All payment endpoints require a valid token. Users register as buyers by default. A buyer can upgrade to a seller at any time.

### Register - `POST /auth/register`

```json
{
  "first_name": "John",
  "last_name": "Doe",
  "email": "john@example.com",
  "password": "yourpassword"
}
```

Response:

```json
{ "message": "registered successfully" }
```

All accounts start as `buyer`.

---

### Login - `POST /auth/login`

```json
{
  "email": "john@example.com",
  "password": "yourpassword"
}
```

Response:

```json
{
  "token": "abc123...",
  "role": "buyer",
  "expires_in": "24h0m0s"
}
```

Store the token and attach it to every request as a Bearer token:

```javascript
headers: {
  'Authorization': `Bearer ${token}`
}
```

---

### Upgrade to Seller - `POST /account/upgrade`

```
POST /account/upgrade
Authorization: Bearer <token>
```

Response:

```json
{ "message": "account upgraded to seller" }
```

The same token is updated in place. No need to log out and back in. Returns `400` if the account is already a seller.

---

### Logout - `POST /auth/logout`

```
POST /auth/logout
Authorization: Bearer <token>
```

Invalidates the token immediately. Returns `204 No Content`.

---

### Token notes

- Tokens expire after 24 hours
- A `401` means the token is missing, invalid, or expired. Log the user out and send them to login
- Roles are `buyer` and `seller`. The role is returned on login and after upgrade

---

## Quick Start - `index.html`

An `index.html` is included in the repo for testing the full flow locally.

It has a hardcoded `txn_ref` -- **this will fail with a duplicate reference error after first use.**

Before each test run, call `POST /checkout` to get a fresh `txn_ref` and update it in the file. Or wire it up dynamically as shown below.

---

## Production Integration

### Step 1 - Include the Interswitch script

```html
<!-- Sandbox -->
<script src="https://newwebpay.qa.interswitchng.com/inline-checkout.js"></script>

<!-- Live -->
<script src="https://newwebpay.interswitchng.com/inline-checkout.js"></script>
```

### Step 2 - Call `/checkout` to get params

```javascript
const res = await fetch('http://localhost:8080/checkout', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  },
  body: JSON.stringify({
    amount: 10000,             // in kobo - N100 = 10000
    cust_name: 'John Doe',
    cust_email: 'john@example.com',
    pay_item_name: 'Product Name',
    site_redirect_url: 'https://yoursite.com'
  })
});

const params = await res.json();
```

### Step 3 - Open the checkout popup

```javascript
window.webpayCheckout({
  ...params,
  onComplete: async function(response) {
    console.log(response);

    if (response.resp !== '00') {
      // payment failed
      return;
    }

    // Step 4 - verify server side
    const verify = await fetch(
      `http://localhost:8080/payment/verify?txn_ref=${response.txnref}&amount=${response.amount}`,
      {
        headers: { 'Authorization': `Bearer ${token}` }
      }
    );
    const result = await verify.json();

    if (result.ResponseCode === '00') {
      // payment confirmed - give value
      console.log('Payment successful');
    } else {
      console.log('Verification failed', result.ResponseDescription);
    }
  }
});
```

---

## Notes

- `amount` is always in kobo. N100 = `10000`
- Every `txn_ref` must be unique. Never reuse one -- Interswitch returns `Z5 Duplicate Transaction Reference`
- Never trust `response.resp` alone. Always call `/payment/verify` before giving value
- `ResponseCode: "00"` = success. Anything else = failed

---

## Test Cards

| Brand | PAN | Expiry | CVV | PIN | OTP |
|---|---|---|---|---|---|
| Verve | 5060990580000217499 | 03/50 | 111 | 1111 | -- |
| Visa | 4000000000000253 | 03/50 | 111 | -- | -- |
| Mastercard | 5123450000000008 | 01/39 | 100 | 1111 | 123456 |

---

## Sandbox vs Live

| | Sandbox | Live |
|---|---|---|
| Script | `newwebpay.qa.interswitchng.com/inline-checkout.js` | `newwebpay.interswitchng.com/inline-checkout.js` |
| Mode param | `TEST` | `LIVE` |
| Backend | dev sets env to sandbox URLs | dev switches to live URLs |
