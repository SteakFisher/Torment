#!/bin/sh
# Runs every request of the brokerage tutorial (docs/src/content/docs/brokerage) against a running server
# at BASE_URL (default http://localhost:8080), sending API_KEY (required) as the bearer token, and exits
# non-zero at the first failed check. Each run uses new users and ids, so it can be rerun against the same
# data. Balances are checked only on the run's own users, since the brokerage's accounts carry every run.
set -eu

BASE_URL=${BASE_URL:-http://localhost:8080}
API_KEY=${API_KEY:?API_KEY is not set}
run=$(date +%s)
user="user:$run"
amira="user:${run}a" omar="user:${run}o"

# post PATH BODY and get PATH print the response body, a space and the status code, on one line.
post() { curl -sS -H "Authorization: Bearer $API_KEY" -X POST -H 'Content-Type: application/json' -d "$2" -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }
get() { curl -sS -H "Authorization: Bearer $API_KEY" -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }

# expect NAME OUTPUT PATTERN fails unless OUTPUT matches the extended regex PATTERN.
expect() {
	if printf '%s' "$2" | grep -Eq -- "$3"; then
		echo "ok   $1"
	else
		echo "FAIL $1: $2" >&2
		exit 1
	fi
}

# id N prints this run's id N; ids grow with N.
id() { printf '%s%02d' "$run" "$1"; }

# account ADDRESS TYPE [EXTRA] prints an account registration, with EXTRA appended as JSON fields.
account() { printf '{"address":"%s","type":"%s"%s}' "$1" "$2" "${3:+,$3}"; }

# p ID SOURCE DESTINATION CURRENCY AMOUNT [GROUP_ID] prints one posting.
p() { printf '{"id":"%s","source":"%s","destination":"%s","currency":"%s","amount":"%s","group_id":"%s"}' "$1" "$2" "$3" "$4" "$5" "${6:-}"; }

# transfer ELEMENT... posts the elements (postings or chains) as one request.
transfer() {
	body=$1
	shift
	for e in "$@"; do body="$body,$e"; done
	post /transfers "{\"postings\":[$body]}"
}

# amount ADDRESS CURRENCY prints the account's balance in currency.
amount() { get "/accounts/$1/balances?currency=$2" | sed -E 's/.*"amount":"([^"]+)".*/\1/'; }

echo "# 1. Chart of accounts"
for code in USD/2 AAPL/6 TSLA/6; do
	expect "register $code" "$(post /currencies "{\"code\":\"$code\"}")" ' (200|201)$'
done
expect "register bank:client-money" "$(post /accounts "$(account bank:client-money asset)")" ' (200|201)$'
expect "register custodian:positions" "$(post /accounts "$(account custodian:positions asset)")" ' (200|201)$'
expect "register revenue:commissions" "$(post /accounts "$(account revenue:commissions revenue)")" ' (200|201)$'
expect "registering again returns 200" "$(post /accounts "$(account bank:client-money asset)")" ' 200$'
expect "register the user's cash with a USD wallet" "$(post /accounts "$(account "$user:cash" liability '"currencies":["USD"]')")" '"wallets":\[\{"currency":"USD","precision":2\}\]\} 201$'
expect "register the user's holdings" "$(post /accounts "$(account "$user:holdings" liability)")" '"wallets":\[\]\} 201$'

echo "# 2. Deposits"
deposit=$(p "$(id 10)" bank:client-money "$user:cash" USD 100000)
expect "deposit 1000 USD" "$(transfer "$deposit")" '"status":"created".* 200$'
expect "the webhook arrives again" "$(transfer "$deposit")" '"status":"exists".* 200$'
expect "the user has 1000 USD" "$(amount "$user:cash" USD)" '^100000$'

echo "# 3. Buying shares"
buy=$(id 20)
expect "buy 1.5 AAPL at 150 with a 1 USD fee" "$(transfer "[$(p "$(id 21)" "$user:cash" bank:client-money USD 22500 "$buy"),$(p "$(id 22)" custodian:positions "$user:holdings" AAPL 1500000 "$buy"),$(p "$(id 23)" "$user:cash" revenue:commissions USD 100 "$buy")]")" '^\{"transfers":\[\[(\{[^}]*"status":"created"\},?){3}\]\]\} 200$'
expect "the user has 774 USD" "$(amount "$user:cash" USD)" '^77400$'
expect "the user has 1.5 AAPL" "$(amount "$user:holdings" AAPL)" '^1500000$'
big=$(id 24)
expect "buying 10 TSLA is refused" "$(transfer "[$(p "$(id 25)" "$user:cash" bank:client-money USD 200000 "$big"),$(p "$(id 26)" custodian:positions "$user:holdings" TSLA 10000000 "$big"),$(p "$(id 27)" "$user:cash" revenue:commissions USD 100 "$big")]")" '"error":"insufficient_funds".*"error":"chain_failed".*"error":"chain_failed".* 200$'
expect "the user still has 774 USD" "$(amount "$user:cash" USD)" '^77400$'
sell=$(id 28)
expect "sell 0.5 AAPL at 160" "$(transfer "[$(p "$(id 29)" "$user:holdings" custodian:positions AAPL 500000 "$sell"),$(p "$(id 30)" bank:client-money "$user:cash" USD 8000 "$sell"),$(p "$(id 31)" "$user:cash" revenue:commissions USD 100 "$sell")]")" '^\{"transfers":\[\[(\{[^}]*"status":"created"\},?){3}\]\]\} 200$'
expect "the user has 853 USD" "$(amount "$user:cash" USD)" '^85300$'
expect "the user has 1 AAPL" "$(amount "$user:holdings" AAPL)" '^1000000$'

echo "# 4. Partial fills"
order=$(id 40)
expect "fill 1: 1 TSLA at 200" "$(transfer "[$(p "$(id 41)" "$user:cash" bank:client-money USD 20000 "$order"),$(p "$(id 42)" custodian:positions "$user:holdings" TSLA 1000000 "$order")]")" '"status":"created".*"status":"created".* 200$'
expect "fill 2: 1 TSLA at 200.50" "$(transfer "[$(p "$(id 43)" "$user:cash" bank:client-money USD 20050 "$order"),$(p "$(id 44)" custodian:positions "$user:holdings" TSLA 1000000 "$order")]")" '"status":"created".*"status":"created".* 200$'
expect "fill 3: 1 TSLA at 199.75, and the fee" "$(transfer "[$(p "$(id 45)" "$user:cash" bank:client-money USD 19975 "$order"),$(p "$(id 46)" custodian:positions "$user:holdings" TSLA 1000000 "$order"),$(p "$(id 47)" "$user:cash" revenue:commissions USD 100 "$order")]")" '"status":"created".*"status":"created".*"status":"created".* 200$'
fills=$(get "/groups/$order/transfers")
expect "the order has 7 transfers" "$(printf '%s' "$fills" | grep -o '"id":' | wc -l | tr -d ' ')" '^7$'
expect "the order's newest transfer is the fee" "$fills" "^\\{\"group_id\":\"$order\",\"transfers\":\\[\\{\"id\":\"$(id 47)\""
expect "the user has 3 TSLA" "$(amount "$user:holdings" TSLA)" '^3000000$'
expect "the user has 251.75 USD" "$(amount "$user:cash" USD)" '^25175$'

echo "# 5. Currency exchange"
expect "register AED/2" "$(post /currencies '{"code":"AED/2"}')" ' (200|201)$'
expect "register fx:position" "$(post /accounts "$(account fx:position equity '"allow_negative":true')")" ' (200|201)$'
expect "deposit 1000 AED" "$(transfer "$(p "$(id 50)" bank:client-money "$user:cash" AED 100000)")" '"status":"created".* 200$'
fx=$(id 51)
expect "exchange 367.25 AED for 100 USD" "$(transfer "[$(p "$(id 52)" "$user:cash" fx:position AED 36725 "$fx"),$(p "$(id 53)" fx:position "$user:cash" USD 10000 "$fx")]")" '"status":"created".*"status":"created".* 200$'
expect "the user has 351.75 USD and 632.75 AED" "$(get "/accounts/$user:cash/balances")" '"currency":"AED","precision":2,"amount":"63275".*"currency":"USD","precision":2,"amount":"35175".* 200$'
settle=$(id 54)
expect "treasury converts at the bank" "$(transfer "[$(p "$(id 55)" fx:position bank:client-money AED 36725 "$settle"),$(p "$(id 56)" bank:client-money fx:position USD 10000 "$settle")]")" '"status":"created".*"status":"created".* 200$'

echo "# 6. Dividends"
for u in "$amira" "$omar"; do
	expect "register $u:cash" "$(post /accounts "$(account "$u:cash" liability)")" ' 201$'
done
dividend=$(id 60)
expect "pay the TSLA dividend to three users" "$(transfer "$(p "$(id 61)" bank:client-money "$user:cash" USD 150 "$dividend")" "$(p "$(id 62)" bank:client-money "$amira:cash" USD 1200 "$dividend")" "$(p "$(id 63)" bank:client-money "$omar:cash" USD 25 "$dividend")")" '^\{"transfers":\[(\{[^}]*"status":"created"\},?){3}\]\} 200$'
expect "the dividend group lists three payments" "$(get "/groups/$dividend/transfers" | grep -o '"id":' | wc -l | tr -d ' ')" '^3$'
expect "the user has 353.25 USD" "$(amount "$user:cash" USD)" '^35325$'

echo "# 7. Withdrawals"
expect "register transit:withdrawals" "$(post /accounts "$(account transit:withdrawals liability)")" ' (200|201)$'
w1=$(id 70)
expect "the user asks to withdraw 100 USD" "$(transfer "$(p "$(id 71)" "$user:cash" transit:withdrawals USD 10000 "$w1")")" '"status":"created".* 200$'
expect "the user has 253.25 USD" "$(amount "$user:cash" USD)" '^25325$'
expect "the bank pays it out" "$(transfer "$(p "$(id 72)" transit:withdrawals bank:client-money USD 10000 "$w1")")" '"status":"created".* 200$'
w2=$(id 73)
expect "the user asks to withdraw 50 USD" "$(transfer "$(p "$(id 74)" "$user:cash" transit:withdrawals USD 5000 "$w2")")" '"status":"created".* 200$'
expect "the bank rejects it, so it goes back" "$(transfer "$(p "$(id 75)" transit:withdrawals "$user:cash" USD 5000 "$w2")")" '"status":"created".* 200$'
expect "the user still has 253.25 USD" "$(amount "$user:cash" USD)" '^25325$'

echo "# 8. Statements"
page=$(get "/accounts/$user:*/transfers?limit=5")
expect "the newest 5 transfers start with the returned withdrawal" "$page" "^\\{\"address\":\"$user:\\*\",\"transfers\":\\[\\{\"id\":\"$(id 75)\".*\"next\":\"[^\"]+\"\\} 200\$"
before=$(printf '%s' "$page" | sed -E 's/.*"next":"([^"]+)".*/\1/')
expect "the next page carries on" "$(get "/accounts/$user:*/transfers?limit=5&before=$before")" "^\\{\"address\":\"$user:\\*\",\"transfers\":\\[\\{\"id\":\"$(id 52)\""
at=$(get "/groups/$(id 10)/transfers" | sed -E 's/.*"timestamp":"([^"]+)".*/\1/')
expect "the balance right after the deposit was 1000 USD" "$(get "/accounts/$user:cash/balances?currency=USD&at=$at")" '"amount":"100000".* 200$'
expect "the user's totals" "$(get "/accounts/$user:*/balances")" '"totals":\[\{"type":"liability","currency":"AAPL","precision":6,"amount":"1000000"\},\{"type":"liability","currency":"AED","precision":2,"amount":"63275"\},\{"type":"liability","currency":"TSLA","precision":6,"amount":"3000000"\},\{"type":"liability","currency":"USD","precision":2,"amount":"25325"\}\]'

echo "# 9. Mistakes"
expect "fill 1 was really at 190: reverse it and post it again" "$(transfer "[$(p "$(id 91)" bank:client-money "$user:cash" USD 20000 "$order"),$(p "$(id 92)" "$user:cash" bank:client-money USD 19000 "$order")]")" '"status":"created".*"status":"created".* 200$'
expect "the user has 263.25 USD" "$(amount "$user:cash" USD)" '^26325$'
expect "the order now has 9 transfers" "$(get "/groups/$order/transfers" | grep -o '"id":' | wc -l | tr -d ' ')" '^9$'
expect "reusing an id with other fields is a conflict" "$(transfer "$(p "$(id 10)" bank:client-money "$user:cash" USD 999)")" '"error":"conflict".* 200$'
